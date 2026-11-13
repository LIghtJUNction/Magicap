package capture

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// EnsureCA never silently replaces an existing identity or an incomplete pair.
func EnsureCA(dir string) (tls.Certificate, string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return tls.Certificate{}, "", err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".ca.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return tls.Certificate{}, "", err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key")
	_, ce := os.Stat(certPath)
	_, ke := os.Stat(keyPath)
	if os.IsNotExist(ce) && os.IsNotExist(ke) {
		key, err := rsa.GenerateKey(rand.Reader, 3072)
		if err != nil {
			return tls.Certificate{}, "", err
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return tls.Certificate{}, "", err
		}
		tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Magicap Local Capture CA", Organization: []string{"Magicap"}}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(5, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			return tls.Certificate{}, "", err
		}
		if err = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
			return tls.Certificate{}, "", err
		}
		if err = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
			return tls.Certificate{}, "", err
		}
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return pair, "", err
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return pair, "", err
	}
	if !cert.IsCA || time.Now().After(cert.NotAfter) {
		return pair, "", fmt.Errorf("invalid or expired CA")
	}
	pair.Leaf = cert
	// Android uses OpenSSL's legacy subject_hash_old (little-endian MD5).
	sum := md5.Sum(cert.RawSubject)
	return pair, fmt.Sprintf("%08x.0", binary.LittleEndian.Uint32(sum[:4])), nil
}
