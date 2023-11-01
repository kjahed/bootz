// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package ownershipvoucher provides helper functions for generating, parsing and verifying ownership vouchers.
package ownershipvoucher

import (
	"crypto"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"time"

	"go.mozilla.org/pkcs7"
)

const (
	ovExpiry = time.Hour * 24 * 365
)

// OwnershipVoucher wraps Inner.
type OwnershipVoucher struct {
	OV Inner `json:"ietf-voucher:voucher"`
}

// Inner defines the Ownership Voucher format. See https://www.rfc-editor.org/rfc/rfc8366.html.
type Inner struct {
	CreatedOn                  string `json:"created-on"`
	ExpiresOn                  string `json:"expires-on"`
	SerialNumber               string `json:"serial-number"`
	Assertion                  string `json:"assertion"`
	PinnedDomainCert           []byte `json:"pinned-domain-cert"`
	DomainCertRevocationChecks bool   `json:"domain-cert-revocation-checks"`
}

// Unmarshal unmarshals the contents of an Ownership Voucher. If a certPool is provided,
// it is used to verify the contents.
func Unmarshal(in []byte, certPool *x509.CertPool) (*OwnershipVoucher, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("ownership voucher is empty")
	}
	p7, err := pkcs7.Parse(in)
	if err != nil {
		return nil, fmt.Errorf("unable to parse into pkcs7 format: %v", err)
	}
	ov := OwnershipVoucher{}
	err = json.Unmarshal(p7.Content, &ov)
	if err != nil {
		return nil, fmt.Errorf("failed unmarshalling ownership voucher: %v", err)
	}
	if certPool != nil {
		if err = p7.VerifyWithChain(certPool); err != nil {
			return nil, fmt.Errorf("failed to verify OV: %v", err)
		}
	}
	return &ov, nil
}

// New generates an Ownership Voucher which is signed by the vendor's CA.
func New(serial string, pdcDER []byte, vendorCACert *x509.Certificate, vendorCAPriv crypto.PrivateKey) ([]byte, error) {
	currentTime := time.Now()
	ov := OwnershipVoucher{
		OV: Inner{
			CreatedOn:        currentTime.Format(time.RFC3339),
			ExpiresOn:        currentTime.Add(ovExpiry).Format(time.RFC3339),
			SerialNumber:     serial,
			PinnedDomainCert: pdcDER,
		},
	}

	ovBytes, err := json.Marshal(ov)
	if err != nil {
		return nil, err
	}

	signedMessage, err := pkcs7.NewSignedData(ovBytes)
	if err != nil {
		return nil, err
	}
	signedMessage.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	signedMessage.SetEncryptionAlgorithm(pkcs7.OIDEncryptionAlgorithmRSA)

	err = signedMessage.AddSigner(vendorCACert, vendorCAPriv, pkcs7.SignerInfoConfig{})
	if err != nil {
		return nil, err
	}

	return signedMessage.Finish()
}
