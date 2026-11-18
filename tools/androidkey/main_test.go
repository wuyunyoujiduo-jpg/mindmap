package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

// TestSealOpen seals by an Ed25519 public key alone and opens by its
// private key: the X25519 key the one stands for is the other's.
func TestSealOpen(t *testing.T) {
	for range 20 {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		in := sealed{Keystore: []byte("keystore bytes"), Password: "pw", Alias: "mymind"}
		blob, err := seal(pub, in)
		if err != nil {
			t.Fatal(err)
		}
		out, err := open(priv, blob)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Keystore, in.Keystore) || out.Password != in.Password || out.Alias != in.Alias {
			t.Fatalf("%+v", out)
		}
		_, other, _ := ed25519.GenerateKey(rand.Reader)
		if _, err := open(other, blob); err == nil {
			t.Fatal("opened by another key")
		}
	}
}
