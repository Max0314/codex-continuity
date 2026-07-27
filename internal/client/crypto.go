package client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

var encryptedMagic = [4]byte{'C', 'C', 'X', '2'}

const encryptedChunkSize = 4 * 1024 * 1024

func EncryptFile(source, destination string, key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cleanup := func(copyErr error) error {
		_ = dst.Close()
		_ = os.Remove(destination)
		return copyErr
	}
	if _, err := dst.Write(encryptedMagic[:]); err != nil {
		return cleanup(err)
	}
	baseNonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(baseNonce); err != nil {
		return cleanup(err)
	}
	if _, err := dst.Write(baseNonce); err != nil {
		return cleanup(err)
	}
	buffer := make([]byte, encryptedChunkSize)
	var counter uint32
	for {
		n, readErr := io.ReadFull(src, buffer)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return cleanup(readErr)
		}
		if n == 0 {
			break
		}
		nonce := chunkNonce(baseNonce, counter)
		ciphertext := aead.Seal(nil, nonce, buffer[:n], encryptedMagic[:])
		if err := binary.Write(dst, binary.BigEndian, uint32(n)); err != nil {
			return cleanup(err)
		}
		if _, err := dst.Write(ciphertext); err != nil {
			return cleanup(err)
		}
		counter++
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	if err := binary.Write(dst, binary.BigEndian, uint32(0)); err != nil {
		return cleanup(err)
	}
	return dst.Close()
}

func DecryptFile(source, destination string, key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	magic := make([]byte, len(encryptedMagic))
	if _, err := io.ReadFull(src, magic); err != nil || string(magic) != string(encryptedMagic[:]) {
		return fmt.Errorf("不是有效的 Codex Continuity 加密包")
	}
	baseNonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(src, baseNonce); err != nil {
		return err
	}
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cleanup := func(copyErr error) error {
		_ = dst.Close()
		_ = os.Remove(destination)
		return copyErr
	}
	var counter uint32
	for {
		var plainSize uint32
		if err := binary.Read(src, binary.BigEndian, &plainSize); err != nil {
			return cleanup(err)
		}
		if plainSize == 0 {
			break
		}
		if plainSize > encryptedChunkSize {
			return cleanup(fmt.Errorf("加密分块尺寸无效"))
		}
		ciphertext := make([]byte, int(plainSize)+aead.Overhead())
		if _, err := io.ReadFull(src, ciphertext); err != nil {
			return cleanup(err)
		}
		plain, err := aead.Open(nil, chunkNonce(baseNonce, counter), ciphertext, encryptedMagic[:])
		if err != nil {
			return cleanup(fmt.Errorf("解密失败，密钥不匹配或文件已损坏"))
		}
		if _, err := dst.Write(plain); err != nil {
			return cleanup(err)
		}
		counter++
	}
	return dst.Close()
}

func chunkNonce(base []byte, counter uint32) []byte {
	nonce := append([]byte(nil), base...)
	offset := len(nonce) - 4
	baseCounter := binary.BigEndian.Uint32(nonce[offset:])
	binary.BigEndian.PutUint32(nonce[offset:], baseCounter+counter)
	return nonce
}
