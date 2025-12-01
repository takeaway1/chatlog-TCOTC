package dat2img

// Implementation based on:
// - https://github.com/tujiaw/wechat_dat_to_image
// - https://github.com/LC044/WeChatMsg/blob/6535ed0/wxManager/decrypt/decrypt_dat.py

import (
	"bytes"
	"crypto/aes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// Format defines the header and extension for different image types
type Format struct {
	Header []byte
	AesKey []byte
	Ext    string
}

var (
	// Common image format definitions
	JPG     = Format{Header: []byte{0xFF, 0xD8, 0xFF}, Ext: "jpg"}
	PNG     = Format{Header: []byte{0x89, 0x50, 0x4E, 0x47}, Ext: "png"}
	GIF     = Format{Header: []byte{0x47, 0x49, 0x46, 0x38}, Ext: "gif"}
	TIFF    = Format{Header: []byte{0x49, 0x49, 0x2A, 0x00}, Ext: "tiff"}
	BMP     = Format{Header: []byte{0x42, 0x4D}, Ext: "bmp"}
	WXGF    = Format{Header: []byte{0x77, 0x78, 0x67, 0x66}, Ext: "wxgf"}
	Formats = []Format{JPG, PNG, GIF, TIFF, BMP, WXGF}

	V4Format1 = Format{Header: []byte{0x07, 0x08, 0x56, 0x31}, AesKey: []byte("cfcd208495d565ef")}
	V4Format2 = Format{Header: []byte{0x07, 0x08, 0x56, 0x32}, AesKey: []byte("0000000000000000")} // FIXME
	V4Formats = []*Format{&V4Format1, &V4Format2}

	// WeChat v4 related constants
	V4XorKey byte = 0x37               // Default XOR key for WeChat v4 dat files
	JpgTail       = []byte{0xFF, 0xD9} // JPG file tail marker
)

// Dat2Image converts WeChat dat file data to image data
// Returns the decoded image data, file extension, and any error encountered
func Dat2Image(data []byte) ([]byte, string, error) {
	log.Debug().Int("data_len", len(data)).Msg("Dat2Image: start processing")
	
	if len(data) < 4 {
		log.Debug().Int("data_len", len(data)).Msg("Dat2Image: data too short")
		return nil, "", fmt.Errorf("data length is too short: %d", len(data))
	}

	// Check if this is a WeChat v4 dat file
	if len(data) >= 6 {
		log.Debug().Str("header", hex.EncodeToString(data[:4])).Msg("Dat2Image: checking v4 format")
		for _, format := range V4Formats {
			if bytes.Equal(data[:4], format.Header) {
				log.Debug().
					Str("format_header", hex.EncodeToString(format.Header)).
					Str("aes_key", hex.EncodeToString(format.AesKey)).
					Int("aes_key_len", len(format.AesKey)).
					Msg("Dat2Image: matched v4 format")
				return Dat2ImageV4(data, format.AesKey)
			}
		}
		log.Debug().Msg("Dat2Image: no v4 format matched")
	}

	// For older WeChat versions, use XOR decryption
	findFormat := func(data []byte, header []byte) bool {
		xorBit := data[0] ^ header[0]
		for i := 0; i < len(header); i++ {
			if data[i]^header[i] != xorBit {
				return false
			}
		}
		return true
	}

	var xorBit byte
	var found bool
	var ext string
	for _, format := range Formats {
		if found = findFormat(data, format.Header); found {
			xorBit = data[0] ^ format.Header[0]
			ext = format.Ext
			break
		}
	}

	if !found {
		log.Debug().Str("header", fmt.Sprintf("%x %x", data[0], data[1])).Msg("Dat2Image: unknown image type")
		return nil, "", fmt.Errorf("unknown image type: %x %x", data[0], data[1])
	}

	log.Debug().Str("ext", ext).Str("xor_bit", fmt.Sprintf("0x%x", xorBit)).Msg("Dat2Image: applying XOR decryption")
	
	// Apply XOR decryption
	out := make([]byte, len(data))
	for i := range data {
		out[i] = data[i] ^ xorBit
	}

	log.Debug().Str("ext", ext).Int("output_len", len(out)).Msg("Dat2Image: XOR decryption completed")
	return out, ext, nil
}

// calculateXorKeyV4 calculates the XOR key for WeChat v4 dat files
// by analyzing the file tail against known JPG ending bytes (FF D9)
func calculateXorKeyV4(data []byte) (byte, error) {
	if len(data) < 2 {
		log.Debug().Int("data_len", len(data)).Msg("calculateXorKeyV4: data too short")
		return 0, fmt.Errorf("data too short to calculate XOR key")
	}

	// Get the last two bytes of the file
	fileTail := data[len(data)-2:]
	log.Debug().Str("file_tail", hex.EncodeToString(fileTail)).Msg("calculateXorKeyV4: analyzing file tail")

	// Assuming it's a JPG file, the tail should be FF D9
	xorKeys := make([]byte, 2)
	for i := 0; i < 2; i++ {
		xorKeys[i] = fileTail[i] ^ JpgTail[i]
	}

	log.Debug().
		Str("xor_key_0", fmt.Sprintf("0x%x", xorKeys[0])).
		Str("xor_key_1", fmt.Sprintf("0x%x", xorKeys[1])).
		Msg("calculateXorKeyV4: calculated XOR keys")

	// Verify that both bytes yield the same XOR key
	if xorKeys[0] == xorKeys[1] {
		log.Debug().Str("xor_key", fmt.Sprintf("0x%x", xorKeys[0])).Msg("calculateXorKeyV4: consistent key found")
		return xorKeys[0], nil
	}

	// If inconsistent, return the first byte as key with a warning
	log.Debug().
		Str("xor_key", fmt.Sprintf("0x%x", xorKeys[0])).
		Msg("calculateXorKeyV4: inconsistent keys, using first byte")
	return xorKeys[0], fmt.Errorf("inconsistent XOR key, using first byte: 0x%x", xorKeys[0])
}

// ScanAndSetXorKey scans a directory for "_t.dat" files to calculate and set
// the global XOR key for WeChat v4 dat files
// Returns the found key and any error encountered
func ScanAndSetXorKey(dirPath string) (byte, error) {
	log.Debug().Str("dir_path", dirPath).Msg("ScanAndSetXorKey: start scanning")
	
	// Walk the directory recursively
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process "_t.dat" files (thumbnail files)
		if !strings.HasSuffix(info.Name(), "_t.dat") {
			return nil
		}

		// Read file content
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Check if it's a WeChat v4 dat file
		if len(data) < 6 || (!bytes.Equal(data[:4], V4Format1.Header) && !bytes.Equal(data[:4], V4Format2.Header)) {
			return nil
		}

		// Parse file header
		if len(data) < 15 {
			return nil
		}

		// Get XOR encryption length
		xorEncryptLen := binary.LittleEndian.Uint32(data[10:14])

		// Get data after header
		fileData := data[15:]

		// Skip if there's no XOR-encrypted part
		if xorEncryptLen == 0 || uint32(len(fileData)) <= uint32(len(fileData))-xorEncryptLen {
			return nil
		}

		// Get XOR-encrypted part
		xorData := fileData[uint32(len(fileData))-xorEncryptLen:]

		// Calculate XOR key
		key, err := calculateXorKeyV4(xorData)
		if err != nil {
			return nil
		}

		// Set global XOR key
		V4XorKey = key
		log.Debug().Str("file", path).Str("xor_key", fmt.Sprintf("0x%x", V4XorKey)).Msg("ScanAndSetXorKey: calculated XOR key")
		log.Debug().Msgf("Calculated WeChat v4 XOR key: 0x%x from file: %s", V4XorKey, path)

		// Stop traversal after finding a valid key
		return filepath.SkipAll
	})

	if err != nil && err != filepath.SkipAll {
		log.Debug().Err(err).Msg("ScanAndSetXorKey: error scanning directory")
		log.Debug().Msgf("Error scanning directory for XOR key: %v", err)
		return V4XorKey, fmt.Errorf("error scanning directory: %v", err)
	}
	log.Debug().Str("xor_key", fmt.Sprintf("0x%x", V4XorKey)).Msg("ScanAndSetXorKey: completed")
	log.Debug().Msgf("WeChat v4 XOR key set to: 0x%x", V4XorKey)

	return V4XorKey, nil
}

func SetAesKey(key string) {
	log.Debug().Str("key", key).Int("key_len", len(key)).Msg("SetAesKey: setting AES key")
	
	if key == "" {
		log.Debug().Msg("SetAesKey: empty key, skipping")
		return
	}
	if len(key) == 16 {
		V4Format2.AesKey = []byte(key)
		log.Debug().Int("aes_key_len", len(V4Format2.AesKey)).Msg("SetAesKey: set 16-byte key directly")
		return
	}
	decoded, err := hex.DecodeString(key)
	if err != nil {
		log.Debug().Err(err).Str("key", key).Msg("SetAesKey: hex decode failed")
		log.Error().Err(err).Msg("invalid aes key")
		return
	}
	V4Format2.AesKey = decoded
	log.Debug().Int("aes_key_len", len(V4Format2.AesKey)).Msg("SetAesKey: set decoded key")
}

// Dat2ImageV4 processes WeChat v4 dat image files
// WeChat v4 uses a combination of AES-ECB and XOR encryption
func Dat2ImageV4(data []byte, aeskey []byte) ([]byte, string, error) {
	log.Debug().
		Int("data_len", len(data)).
		Int("aes_key_len", len(aeskey)).
		Str("aes_key", hex.EncodeToString(aeskey)).
		Msg("Dat2ImageV4: start processing")
	
	if len(data) < 15 {
		log.Debug().Int("data_len", len(data)).Msg("Dat2ImageV4: data too short")
		return nil, "", fmt.Errorf("data length is too short for WeChat v4 format: %d", len(data))
	}

	// Parse dat file header:
	// - 6 bytes: 0x07085631 or 0x07085632 (dat file identifier)
	// - 4 bytes: int (little-endian) AES-ECB128 encryption length
	// - 4 bytes: int (little-endian) XOR encryption length
	// - 1 byte:  0x01 (unknown)

	// Read AES encryption length
	aesEncryptLen := binary.LittleEndian.Uint32(data[6:10])
	// Read XOR encryption length
	xorEncryptLen := binary.LittleEndian.Uint32(data[10:14])

	log.Debug().
		Uint32("aes_encrypt_len", aesEncryptLen).
		Uint32("xor_encrypt_len", xorEncryptLen).
		Msg("Dat2ImageV4: parsed header")

	// Data after header
	fileData := data[15:]

	// AES encrypted part (max 1KB)
	// Round up to multiple of 16 bytes for AES block size
	aesEncryptLen0 := (aesEncryptLen)/16*16 + 16
	if aesEncryptLen0 > uint32(len(fileData)) {
		aesEncryptLen0 = uint32(len(fileData))
	}
	
	log.Debug().
		Uint32("aes_encrypt_len0", aesEncryptLen0).
		Int("file_data_len", len(fileData)).
		Msg("Dat2ImageV4: calculated AES block size")

	// Decrypt AES part
	log.Debug().Uint32("decrypt_len", aesEncryptLen0).Msg("Dat2ImageV4: decrypting AES part")
	aesDecryptedData, err := decryptAESECB(fileData[:aesEncryptLen0], aeskey)
	if err != nil {
		log.Debug().Err(err).Msg("Dat2ImageV4: AES decrypt failed")
		log.Info().Msg("[#236]AES decrypt error: " + err.Error())
		return nil, "", fmt.Errorf("AES decrypt error: %v", err)
	}
	log.Debug().Int("decrypted_len", len(aesDecryptedData)).Msg("Dat2ImageV4: AES decryption completed")

	// Prepare result buffer
	var result []byte

	// Add decrypted AES part (remove padding if necessary)
	if len(aesDecryptedData) > int(aesEncryptLen) {
		result = append(result, aesDecryptedData[:aesEncryptLen]...)
	} else {
		result = append(result, aesDecryptedData...)
	}

	// Add unencrypted middle part
	middleStart := aesEncryptLen0
	middleEnd := uint32(len(fileData)) - xorEncryptLen
	if middleStart < middleEnd {
		log.Debug().
			Uint32("middle_start", middleStart).
			Uint32("middle_end", middleEnd).
			Int("middle_len", int(middleEnd-middleStart)).
			Msg("Dat2ImageV4: adding unencrypted middle part")
		result = append(result, fileData[middleStart:middleEnd]...)
	}

	// Process XOR-encrypted part (file tail)
	if xorEncryptLen > 0 && middleEnd < uint32(len(fileData)) {
		xorData := fileData[middleEnd:]
		log.Debug().
			Int("xor_data_len", len(xorData)).
			Str("xor_key", fmt.Sprintf("0x%x", V4XorKey)).
			Msg("Dat2ImageV4: decrypting XOR part")

		// Apply XOR decryption using global key
		xorDecrypted := make([]byte, len(xorData))
		for i := range xorData {
			xorDecrypted[i] = xorData[i] ^ V4XorKey
		}

		result = append(result, xorDecrypted...)
		log.Debug().Int("xor_decrypted_len", len(xorDecrypted)).Msg("Dat2ImageV4: XOR decryption completed")
	}

	// Identify image type from decrypted data
	log.Debug().Int("result_len", len(result)).Msg("Dat2ImageV4: identifying image type")
	if len(result) >= 4 {
		log.Debug().Str("result_header", hex.EncodeToString(result[:4])).Msg("Dat2ImageV4: result header")
	}
	
	imgType := ""
	for _, format := range Formats {
		if len(result) >= len(format.Header) && bytes.Equal(result[:len(format.Header)], format.Header) {
			imgType = format.Ext
			log.Debug().Str("img_type", imgType).Msg("Dat2ImageV4: matched image format")
			break
		}
	}

	if imgType == "wxgf" {
		log.Debug().Msg("Dat2ImageV4: processing wxgf format")
		return Wxam2pic(result)
	}

	if imgType == "" {
		log.Debug().Msg("Dat2ImageV4: unknown image type after decryption")
		return nil, "", fmt.Errorf("unknown image type after decryption")
	}
	log.Debug().Str("img_type", imgType).Int("result_len", len(result)).Msg("Dat2ImageV4: decoding completed")
	log.Info().Msg("Dat2ImageV4 decoded image type: " + imgType)
	return result, imgType, nil
}

// decryptAESECB decrypts data using AES in ECB mode
func decryptAESECB(data, key []byte) ([]byte, error) {
	log.Debug().
		Int("data_len", len(data)).
		Int("key_len", len(key)).
		Msg("decryptAESECB: start")
	
	if len(data) == 0 {
		log.Debug().Msg("decryptAESECB: empty data")
		return nil, nil
	}

	// Create AES cipher
	cipher, err := aes.NewCipher(key)
	if err != nil {
		log.Debug().
			Err(err).
			Int("key_len", len(key)).
			Msg("decryptAESECB: failed to create cipher")
		return nil, err
	}

	// Ensure data length is a multiple of block size
	if len(data)%aes.BlockSize != 0 {
		log.Debug().
			Int("data_len", len(data)).
			Int("block_size", aes.BlockSize).
			Msg("decryptAESECB: data length not multiple of block size")
		return nil, fmt.Errorf("data length is not a multiple of block size")
	}

	decrypted := make([]byte, len(data))

	// ECB mode requires block-by-block decryption
	for bs, be := 0, aes.BlockSize; bs < len(data); bs, be = bs+aes.BlockSize, be+aes.BlockSize {
		cipher.Decrypt(decrypted[bs:be], data[bs:be])
	}

	// Handle PKCS#7 padding
	padding := int(decrypted[len(decrypted)-1])
	if padding > 0 && padding <= aes.BlockSize {
		// Validate padding
		valid := true
		for i := len(decrypted) - padding; i < len(decrypted); i++ {
			if decrypted[i] != byte(padding) {
				valid = false
				break
			}
		}

		if valid {
			log.Debug().
				Int("padding", padding).
				Int("original_len", len(decrypted)).
				Int("final_len", len(decrypted)-padding).
				Msg("decryptAESECB: removed valid padding")
			return decrypted[:len(decrypted)-padding], nil
		}
		log.Debug().Int("padding", padding).Msg("decryptAESECB: invalid padding, keeping data as-is")
	}

	log.Debug().Int("decrypted_len", len(decrypted)).Msg("decryptAESECB: completed without padding removal")
	return decrypted, nil
}
