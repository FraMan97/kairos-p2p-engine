package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
)

func GenerateKeyPair() error {
	keysDir := filepath.Dir(config.PrivateKeyDir)

	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return fmt.Errorf("error creating keys folder: %w", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("error generating keys: %w", err)
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("error marshaling private key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	err = os.WriteFile(config.PrivateKeyDir, privateKeyPEM, 0600)
	if err != nil {
		return fmt.Errorf("error saving private key: %w", err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	err = os.WriteFile(config.PublicKeyDir, publicKeyPEM, 0644)
	if err != nil {
		return fmt.Errorf("error saving public key: %w", err)
	}

	log.Println("[Keys] - Keys generated successfully!")

	publicKey, err = GetPublicKey()
	if err != nil {
		return err
	}
	config.PublicKey = publicKey
	privateKey, err = GetPrivateKey()
	if err != nil {
		return err
	}
	config.PrivateKey = privateKey
	return nil
}

func SignMessage(message []byte) ([]byte, error) {
	privateKeyPEM, err := os.ReadFile(config.PrivateKeyDir)
	if err != nil {
		return nil, fmt.Errorf("error reading private key: %w", err)
	}

	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKeyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing private key: %w", err)
	}

	privateKey, ok := privateKeyInterface.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 private key")
	}

	return ed25519.Sign(privateKey, message), nil
}

func GetPublicKey() ([]byte, error) {
	return os.ReadFile(config.PublicKeyDir)
}

func GetPrivateKey() ([]byte, error) {
	return os.ReadFile(config.PrivateKeyDir)
}

func VerifySignature(message []byte, signature []byte, publicKey []byte) (bool, error) {
	block, _ := pem.Decode(publicKey)
	if block == nil {
		return false, fmt.Errorf("failed to decode PEM block")
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("error parsing public key: %w", err)
	}

	pubKey, ok := publicKeyInterface.(ed25519.PublicKey)
	if !ok {
		return false, fmt.Errorf("not an Ed25519 public key")
	}

	return ed25519.Verify(pubKey, message, signature), nil
}

func GenerateRandomAESKey() []byte {
	aesKey := make([]byte, 32)
	rand.Read(aesKey)
	return aesKey
}

func EncryptGCM(data, key []byte) ([]byte, error) {
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return gcm.Seal(nonce, nonce, data, nil), nil
}

func DecryptGCM(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("NewCipher error: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("NewGCM error: %v", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext is too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func KeysExist() bool {
	if _, err := os.Stat(config.PrivateKeyDir); os.IsNotExist(err) {
		return false
	}
	return true
}
