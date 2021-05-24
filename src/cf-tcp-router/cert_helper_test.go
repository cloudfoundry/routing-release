package main_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

type CertType int

const (
	isCA CertType = iota
	isServer
	isClient
)

func createCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %s", err)
	}

	tmpl, err := createCertTemplate(isCA)
	if err != nil {
		return nil, nil, fmt.Errorf("create cert template: %s", err)
	}

	caDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &caPriv.PublicKey, caPriv)
	if err != nil {
		return nil, nil, fmt.Errorf("creating certificate: %s", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing ca cert: %s", err)
	}

	return caCert, caPriv, nil
}

func createCertificate(rootCert *x509.Certificate, caPriv *ecdsa.PrivateKey, certType CertType) (tls.Certificate, error) {
	certPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate key: %s", err)
	}

	certTemplate, err := createCertTemplate(certType)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create cert template: %s", err)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &certTemplate, rootCert, &certPriv.PublicKey, caPriv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("x509 create certificate: %s", err)
	}

	privBytes, err := x509.MarshalECPrivateKey(certPriv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal ec private key: %s", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "EC PRIVATE KEY", Bytes: privBytes,
	})

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: certDER,
	})

	x509KeyPair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("making x509 key pair: %s", err)
	}

	return x509KeyPair, nil
}

func createCertTemplate(certType CertType) (x509.Certificate, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return x509.Certificate{}, fmt.Errorf("random int: %s", err)
	}

	tmpl := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"TESTING"}},
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	switch certType {
	case isCA:
		tmpl.IsCA = true
		tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	case isServer:
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	case isClient:
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	return tmpl, err
}
