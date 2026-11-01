package hyperliquid

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/crypto/sha3"
)

// keccak256 computes the Keccak-256 hash (not SHA3-256).
func keccak256(data ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()

	for _, d := range data {
		h.Write(d)
	}

	return h.Sum(nil)
}

// SignatureResult holds the ECDSA signature components for the exchange API.
type SignatureResult struct {
	R string `json:"r"`
	S string `json:"s"`
	V int    `json:"v"`
}

// PrivateKeyBytes parses a hex private key string (with or without 0x prefix) into raw bytes.
func PrivateKeyBytes(hexKey string) ([]byte, error) {
	hexKey = strings.TrimPrefix(hexKey, "0x")

	data, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key hex: %w", err)
	}

	if len(data) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes, got %d", len(data))
	}

	return data, nil
}

// DeriveAddress computes the EVM address (20 bytes) from a private key.
func DeriveAddress(privKeyBytes []byte) string {
	priv := secp256k1.PrivKeyFromBytes(privKeyBytes)
	pubKey := priv.PubKey()

	pubBytes := pubKey.SerializeUncompressed()[1:] // strip 0x04 prefix → 64 bytes
	hash := keccak256(pubBytes)

	return fmt.Sprintf("0x%s", hex.EncodeToString(hash[12:]))
}

// SignL1Action signs an L1 action using the Hyperliquid EIP-712 phantom-agent scheme.
// action must be a struct with msgpack tags (field order is critical).
func SignL1Action(privKeyBytes []byte, action any, vaultAddress string, nonce int64, expiresAfter *int64, isMainnet bool) (SignatureResult, error) {
	hash := actionHash(action, vaultAddress, nonce, expiresAfter)

	phantomAgent := constructPhantomAgent(hash, isMainnet)

	digest, err := eip712Digest(phantomAgent)
	if err != nil {
		return SignatureResult{}, fmt.Errorf("failed to compute EIP-712 digest: %w", err)
	}

	return signDigest(privKeyBytes, digest)
}

// actionHash constructs the msgpack action hash that goes into the phantom agent.
func actionHash(action any, vaultAddress string, nonce int64, expiresAfter *int64) []byte {
	var buf bytes.Buffer

	enc := msgpack.NewEncoder(&buf)
	enc.UseCompactInts(true)

	if err := enc.Encode(action); err != nil {
		panic(fmt.Sprintf("failed to msgpack-encode action: %v", err))
	}

	data := buf.Bytes()

	// Python msgpack uses str8 for strings <256 bytes; Go's vmihailenco uses str16.
	// Convert to match exactly for signature compatibility.
	data = convertStr16ToStr8(data)

	// Append nonce as 8 bytes big-endian.
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, uint64(nonce))
	data = append(data, nonceBytes...)

	// Append vault address marker.
	if vaultAddress == "" {
		data = append(data, 0x00)
	} else {
		data = append(data, 0x01)
		data = append(data, addressToBytes(vaultAddress)...)
	}

	// Append expiresAfter if provided.
	if expiresAfter != nil {
		data = append(data, 0x00)
		expiresBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(expiresBytes, uint64(*expiresAfter))
		data = append(data, expiresBytes...)
	}

	return keccak256(data)
}

func addressToBytes(address string) []byte {
	address = strings.TrimPrefix(address, "0x")

	bytes, _ := hex.DecodeString(address)

	return bytes
}

// constructPhantomAgent builds the EIP-712 message: {source, connectionId}.
func constructPhantomAgent(hash []byte, isMainnet bool) map[string]any {
	source := "b" // testnet
	if isMainnet {
		source = "a" // mainnet
	}

	return map[string]any{
		"source":       source,
		"connectionId": hash,
	}
}

// eip712Digest computes the final hash to sign:
// keccak256(0x1901 || domainSeparator || structHash)
func eip712Digest(phantomAgent map[string]any) ([]byte, error) {
	// Domain: name="Exchange", version="1", chainId=1337, verifyingContract=0x0
	domainTypeStr := "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
	domainTypeHash := keccak256([]byte(domainTypeStr))

	domainEncoded := domainTypeHash
	domainEncoded = append(domainEncoded, keccak256([]byte("Exchange"))...)
	domainEncoded = append(domainEncoded, keccak256([]byte("1"))...)

	chainIdBytes := make([]byte, 32)
	new(big.Int).SetInt64(1337).FillBytes(chainIdBytes)
	domainEncoded = append(domainEncoded, chainIdBytes...)

	zeroAddr := make([]byte, 32) // 0x000...000 left-padded
	domainEncoded = append(domainEncoded, zeroAddr...)
	domainSeparator := keccak256(domainEncoded)

	// Agent type
	agentTypeStr := "Agent(string source,bytes32 connectionId)"
	agentTypeHash := keccak256([]byte(agentTypeStr))

	sourceStr, _ := phantomAgent["source"].(string)
	connectionId, _ := phantomAgent["connectionId"].([]byte)

	agentEncoded := agentTypeHash
	agentEncoded = append(agentEncoded, keccak256([]byte(sourceStr))...)

	// connectionId is already 32 bytes (keccak hash)
	if len(connectionId) != 32 {
		return nil, fmt.Errorf("connectionId must be 32 bytes, got %d", len(connectionId))
	}

	agentEncoded = append(agentEncoded, connectionId...)
	structHash := keccak256(agentEncoded)

	// Final digest: 0x1901 + domain + struct
	return keccak256([]byte{0x19, 0x01}, domainSeparator, structHash), nil
}

// signDigest signs a 32-byte hash with secp256k1 and returns the Ethereum-style signature.
func signDigest(privKeyBytes []byte, digest []byte) (SignatureResult, error) {
	priv := secp256k1.PrivKeyFromBytes(privKeyBytes)

	// SignCompact returns 65 bytes: R(32) + S(32) + headerByte(1).
	// headerByte = 27 + recoveryID for uncompressed keys.
	compactSig := ecdsa.SignCompact(priv, digest, false)

	r := new(big.Int).SetBytes(compactSig[0:32])
	s := new(big.Int).SetBytes(compactSig[32:64])
	v := int(compactSig[64]) // already 27 or 28

	return SignatureResult{
		R: fmt.Sprintf("0x%s", hex.EncodeToString(r.Bytes())),
		S: fmt.Sprintf("0x%s", hex.EncodeToString(s.Bytes())),
		V: v,
	}, nil
}

// convertStr16ToStr8 converts msgpack str16 headers (0xda + 2-byte len) to str8
// (0xd9 + 1-byte len) for strings shorter than 256 bytes, matching Python's
// msgpack library output. This is required for signature compatibility.
func convertStr16ToStr8(data []byte) []byte {
	result := make([]byte, 0, len(data))
	pos := 0

	for pos < len(data) {
		consumed := walkMsgpackValue(data, pos, &result)

		if consumed <= 0 {
			result = append(result, data[pos:]...)
			break
		}

		pos += consumed
	}

	return result
}

func walkMsgpackValue(data []byte, pos int, result *[]byte) int {
	if pos >= len(data) {
		return 0
	}

	b := data[pos]
	remaining := len(data) - pos

	// Fixed-length single-byte types
	if b <= 0x7f || b >= 0xe0 || (b >= 0xc0 && b <= 0xc3) {
		*result = append(*result, b)
		return 1
	}

	// fixstr
	if b >= 0xa0 && b <= 0xbf {
		n := int(b & 0x1f)
		total := 1 + n
		if remaining < total {
			return 0
		}
		*result = append(*result, data[pos:pos+total]...)
		return total
	}

	// fixmap
	if b >= 0x80 && b <= 0x8f {
		count := int(b & 0x0f)
		*result = append(*result, b)
		consumed := 1
		for i := 0; i < count*2; i++ {
			c := walkMsgpackValue(data, pos+consumed, result)
			if c <= 0 {
				return 0
			}
			consumed += c
		}
		return consumed
	}

	// fixarray
	if b >= 0x90 && b <= 0x9f {
		count := int(b & 0x0f)
		*result = append(*result, b)
		consumed := 1
		for i := 0; i < count; i++ {
			c := walkMsgpackValue(data, pos+consumed, result)
			if c <= 0 {
				return 0
			}
			consumed += c
		}
		return consumed
	}

	switch b {
	case 0xca:
		return copyFixed(data, pos, 5, result)
	case 0xcb:
		return copyFixed(data, pos, 9, result)
	case 0xcc:
		return copyFixed(data, pos, 2, result)
	case 0xcd:
		return copyFixed(data, pos, 3, result)
	case 0xce:
		return copyFixed(data, pos, 5, result)
	case 0xcf:
		return copyFixed(data, pos, 9, result)
	case 0xd0:
		return copyFixed(data, pos, 2, result)
	case 0xd1:
		return copyFixed(data, pos, 3, result)
	case 0xd2:
		return copyFixed(data, pos, 5, result)
	case 0xd3:
		return copyFixed(data, pos, 9, result)
	case 0xd4:
		return copyFixed(data, pos, 3, result)
	case 0xd5:
		return copyFixed(data, pos, 4, result)
	case 0xd6:
		return copyFixed(data, pos, 6, result)
	case 0xd7:
		return copyFixed(data, pos, 10, result)
	case 0xd8:
		return copyFixed(data, pos, 18, result)
	case 0xc4:
		return copyVarLen(data, pos, 1, result)
	case 0xc5:
		return copyVarLen(data, pos, 2, result)
	case 0xc6:
		return copyVarLen(data, pos, 4, result)
	case 0xc7:
		return copyExtVarLen(data, pos, 1, result)
	case 0xc8:
		return copyExtVarLen(data, pos, 2, result)
	case 0xc9:
		return copyExtVarLen(data, pos, 4, result)
	case 0xd9:
		return copyVarLen(data, pos, 1, result)
	case 0xda:
		if remaining < 3 {
			return 0
		}
		length := (int(data[pos+1]) << 8) | int(data[pos+2])
		total := 3 + length
		if remaining < total {
			return 0
		}
		if length < 256 {
			*result = append(*result, 0xd9)
			*result = append(*result, byte(length))
			*result = append(*result, data[pos+3:pos+total]...)
		} else {
			*result = append(*result, data[pos:pos+total]...)
		}
		return total
	case 0xdb:
		return copyVarLen(data, pos, 4, result)
	case 0xdc:
		if remaining < 3 {
			return 0
		}
		count := (int(data[pos+1]) << 8) | int(data[pos+2])
		*result = append(*result, data[pos:pos+3]...)
		consumed := 3
		for i := 0; i < count; i++ {
			c := walkMsgpackValue(data, pos+consumed, result)
			if c <= 0 {
				return 0
			}
			consumed += c
		}
		return consumed
	case 0xdd:
		if remaining < 5 {
			return 0
		}
		count := (int(data[pos+1]) << 24) | (int(data[pos+2]) << 16) | (int(data[pos+3]) << 8) | int(data[pos+4])
		*result = append(*result, data[pos:pos+5]...)
		consumed := 5
		for i := 0; i < count; i++ {
			c := walkMsgpackValue(data, pos+consumed, result)
			if c <= 0 {
				return 0
			}
			consumed += c
		}
		return consumed
	case 0xde:
		if remaining < 3 {
			return 0
		}
		count := (int(data[pos+1]) << 8) | int(data[pos+2])
		*result = append(*result, data[pos:pos+3]...)
		consumed := 3
		for i := 0; i < count*2; i++ {
			c := walkMsgpackValue(data, pos+consumed, result)
			if c <= 0 {
				return 0
			}
			consumed += c
		}
		return consumed
	case 0xdf:
		if remaining < 5 {
			return 0
		}
		count := (int(data[pos+1]) << 24) | (int(data[pos+2]) << 16) | (int(data[pos+3]) << 8) | int(data[pos+4])
		*result = append(*result, data[pos:pos+5]...)
		consumed := 5
		for i := 0; i < count*2; i++ {
			c := walkMsgpackValue(data, pos+consumed, result)
			if c <= 0 {
				return 0
			}
			consumed += c
		}
		return consumed
	default:
		*result = append(*result, b)
		return 1
	}
}

func copyFixed(data []byte, pos, size int, result *[]byte) int {
	if len(data)-pos < size {
		return 0
	}
	*result = append(*result, data[pos:pos+size]...)
	return size
}

func copyVarLen(data []byte, pos, lenBytes int, result *[]byte) int {
	headerSize := 1 + lenBytes
	if len(data)-pos < headerSize {
		return 0
	}
	length := readLen(data, pos+1, lenBytes)
	total := headerSize + length
	if len(data)-pos < total {
		return 0
	}
	*result = append(*result, data[pos:pos+total]...)
	return total
}

func copyExtVarLen(data []byte, pos, lenBytes int, result *[]byte) int {
	headerSize := 1 + lenBytes + 1
	if len(data)-pos < headerSize {
		return 0
	}
	length := readLen(data, pos+1, lenBytes)
	total := headerSize + length
	if len(data)-pos < total {
		return 0
	}
	*result = append(*result, data[pos:pos+total]...)
	return total
}

func readLen(data []byte, pos, size int) int {
	switch size {
	case 1:
		return int(data[pos])
	case 2:
		return (int(data[pos]) << 8) | int(data[pos+1])
	case 4:
		return (int(data[pos]) << 24) | (int(data[pos+1]) << 16) | (int(data[pos+2]) << 8) | int(data[pos+3])
	default:
		return 0
	}
}
