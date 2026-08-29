// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package trust

import (
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	msg := []byte("evidence object bytes")
	sig := Sign(kp.Private, msg)

	if !Verify(kp.Public, msg, sig) {
		t.Fatal("Verify rejected a valid signature")
	}

	tampered := append([]byte(nil), msg...)
	tampered[0] ^= 0xff
	if Verify(kp.Public, tampered, sig) {
		t.Fatal("Verify accepted a signature over tampered bytes")
	}

	tamperedSig := append([]byte(nil), sig...)
	tamperedSig[0] ^= 0xff
	if Verify(kp.Public, msg, tamperedSig) {
		t.Fatal("Verify accepted a tampered signature")
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	fp1 := FingerprintOf(kp.Public)
	fp2 := FingerprintOf(kp.Public)
	if fp1 != fp2 {
		t.Fatalf("Fingerprint not deterministic: %s != %s", fp1, fp2)
	}

	other, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	if FingerprintOf(other.Public) == fp1 {
		t.Fatal("two distinct generated keys produced the same fingerprint")
	}
}

func TestGenerateEphemeralKeyValidityCapped(t *testing.T) {
	kp, err := GenerateEphemeralKey(24 * time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	if got := kp.NotAfter.Sub(kp.NotBefore); got > MaxKeyValidity {
		t.Fatalf("validity window %s exceeds MaxKeyValidity %s", got, MaxKeyValidity)
	}
}

func TestDestroyZeroesPrivateKey(t *testing.T) {
	kp, err := GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	kp.Destroy()
	for i, b := range kp.Private {
		if b != 0 {
			t.Fatalf("kp.Private[%d] = %d, want 0 after Destroy", i, b)
		}
	}
}
