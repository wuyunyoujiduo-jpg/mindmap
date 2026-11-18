// Command androidkey keeps the key that signs the Android app in the
// repository, sealed so that only the workflow opens it: the repository is
// public, and GitHub's secrets cannot be written from where the key was
// made. It is sealed (NaCl's anonymous box) to the updates' key
// (mygo.json's updates.publicKey, an Ed25519 key), taken as the X25519
// key it stands for, as age does with SSH's Ed25519 keys; the workflow
// opens it with MYGO_UPDATER_PRIVATE_KEY, the secret it signs the updates
// with already. Whoever has that secret could ship an update to every
// Mac, so the Android key is no weaker kept behind it.
//
//	go run ./tools/androidkey seal <public key> <keystore> <password> <alias> <out>
//	MYGO_UPDATER_PRIVATE_KEY=… go run ./tools/androidkey open <sealed> <dir>
//
// open writes <dir>/upload.jks, <dir>/password and <dir>/alias.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// sealed is what the box holds.
type sealed struct {
	Keystore []byte `json:"keystore"`
	Password string `json:"password"`
	Alias    string `json:"alias"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "androidkey:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch {
	case len(args) == 6 && args[0] == "seal":
		pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(args[1]))
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return errors.New("not an Ed25519 public key")
		}
		ks, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		out, err := seal(pub, sealed{Keystore: ks, Password: args[3], Alias: args[4]})
		if err != nil {
			return err
		}
		return os.WriteFile(args[5], []byte(base64.StdEncoding.EncodeToString(out)+"\n"), 0o644)
	case len(args) == 3 && args[0] == "open":
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("MYGO_UPDATER_PRIVATE_KEY")))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			return errors.New("MYGO_UPDATER_PRIVATE_KEY is not an Ed25519 private key")
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		blob, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return err
		}
		s, err := open(ed25519.PrivateKey(key), blob)
		if err != nil {
			return err
		}
		dir := args[2]
		for name, b := range map[string][]byte{"upload.jks": s.Keystore, "password": []byte(s.Password), "alias": []byte(s.Alias)} {
			if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("usage: androidkey seal <public key> <keystore> <password> <alias> <out> | open <sealed> <dir>")
}

func seal(pub ed25519.PublicKey, s sealed) ([]byte, error) {
	to, err := montgomery(pub)
	if err != nil {
		return nil, err
	}
	msg, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return box.SealAnonymous(nil, msg, to, rand.Reader)
}

func open(key ed25519.PrivateKey, blob []byte) (sealed, error) {
	var s sealed
	priv, pub := x25519(key)
	msg, ok := box.OpenAnonymous(nil, blob, pub, priv)
	if !ok {
		return s, errors.New("not sealed to this key")
	}
	return s, json.Unmarshal(msg, &s)
}

// x25519 is the X25519 key pair an Ed25519 private key stands for: the
// scalar is the first half of the seed's SHA-512, as Ed25519's own, and
// the public key is derived from it.
func x25519(key ed25519.PrivateKey) (priv, pub *[32]byte) {
	h := sha512.Sum512(key.Seed())
	priv, pub = new([32]byte), new([32]byte)
	copy(priv[:], h[:32])
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	p, _ := curve25519.X25519(priv[:], curve25519.Basepoint)
	copy(pub[:], p)
	return priv, pub
}

// montgomery is the X25519 public key of an Ed25519 one: u = (1+y)/(1-y)
// mod 2^255-19, y being the point's coordinate as the key says it.
func montgomery(pub ed25519.PublicKey) (*[32]byte, error) {
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	le := make([]byte, 32)
	for i := range 32 {
		le[i] = pub[31-i]
	}
	le[0] &= 0x7f // the sign of x
	y := new(big.Int).SetBytes(le)
	if y.Cmp(p) >= 0 {
		return nil, errors.New("not a point")
	}
	one := big.NewInt(1)
	den := new(big.Int).Mod(new(big.Int).Sub(one, y), p)
	if den.Sign() == 0 {
		return nil, errors.New("not a point")
	}
	u := new(big.Int).Mul(new(big.Int).Add(one, y), new(big.Int).ModInverse(den, p))
	u.Mod(u, p)
	b := u.FillBytes(make([]byte, 32))
	out := new([32]byte)
	for i := range 32 {
		out[i] = b[31-i]
	}
	return out, nil
}
