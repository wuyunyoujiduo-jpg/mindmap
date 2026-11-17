// Package vault encrypts what of the library is a key or a password, so
// that it is kept and synced as ciphertext.
//
// A vault is a pair of keys. The public one is kept as it is, and seals a
// secret with no password asked: what is collected is sealed as it is
// kept. The private one opens them, and is kept locked twice, by the
// user's second password and by a recovery code, each through Argon2id.
// A secret sealed is a token in the text it was in, ⟦mm1.<key>.<data>⟧,
// the data NaCl's anonymous box (X25519, XSalsa20-Poly1305) in base64.
//
// The keys are files of a folder, <library>/vault/<key>.json, synced with
// the rest. They are never deleted, and a vault turned off is one marked
// so: a computer that sealed with a key before it heard of that is still
// to open what it sealed. Two computers that each set a vault up before
// they synced leave two files, each opening what its key sealed; a file
// changed on both is kept twice by the sync, and both are read.
//
// Nothing here is of this package's invention but the layout: the
// primitives are golang.org/x/crypto's.
package vault

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

// Token finds the secrets sealed in a text. One cut short, by a text
// cut, is found too: it is no more to be read than a whole one.
var Token = regexp.MustCompile(`⟦mm1\.[0-9a-f]*\.?[A-Za-z0-9_\-]*⟧?`)

const tokenStart, tokenEnd = "⟦mm1.", "⟧"

// Has tells whether s holds a sealed secret.
func Has(s string) bool { return strings.Contains(s, tokenStart) }

// MinPassword is the least a second password may be: what is sealed is
// on GitHub, where a short one is tried in no time.
const MinPassword = 8

// Errors of a vault.
var (
	ErrWrong  = errors.New("密码不对")
	ErrCode   = errors.New("恢复码不对")
	ErrShort  = errors.New("密码至少 8 位")
	ErrLocked = errors.New("要先解锁")
	ErrOff    = errors.New("还没有设置二次密码")
)

// cost is Argon2id's: RFC 9106's second choice, 64 MiB and three passes.
// The tests take less.
var cost = kdf{Time: 3, Memory: 64 * 1024, Threads: 4}

type kdf struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"` // KiB
	Threads uint8  `json:"threads"`
}

// A wrap is the private key locked by a password or a code.
type wrap struct {
	KDF  string `json:"kdf"` // argon2id
	Salt []byte `json:"salt"`
	kdf
	Box []byte `json:"box"` // the nonce, then the key sealed
}

// A lock is a key's file.
type lock struct {
	Version  int       `json:"version"`
	ID       string    `json:"id"`
	Public   []byte    `json:"public"`
	Password wrap      `json:"password"`
	Recovery wrap      `json:"recovery"`
	Created  time.Time `json:"created"`
	Changed  time.Time `json:"changed"`
	// Off says the vault was turned off: the key seals nothing more, and
	// still opens what it sealed.
	Off bool `json:"off,omitempty"`

	file string // its name in the folder
}

// Vault is the keys of a library.
type Vault struct {
	// Dir is the folder of the keys' files.
	Dir string

	mu    sync.Mutex
	locks []*lock
	keys  map[string]*[32]byte // the private keys opened, by ID
	// memo is the token of each secret opened or sealed since unlocked, so
	// that a text shown and kept again is kept as it was, to the byte.
	memo map[string]string
	// spare is where the keys' files are kept a second time, on this
	// computer alone.
	spare string
	// locked counts the times Lock was called: an unlock under way when
	// one was does not open what was locked after it began.
	locked int
}

// Open reads the keys of a folder, which need not exist.
func Open(dir string) *Vault {
	v := &Vault{Dir: dir}
	v.Reload()
	return v
}

// Reload reads the keys' files again, as after a sync. What was unlocked
// stays so.
func (v *Vault) Reload() {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.locks = readLocks(v.Dir)
	v.restore()
}

func readLocks(dir string) []*lock {
	var locks []*lock
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var l lock
		if json.Unmarshal(data, &l) != nil || l.ID == "" || len(l.Public) != 32 {
			continue
		}
		l.file = e.Name()
		locks = append(locks, &l)
	}
	// The oldest first: the one every computer takes to seal with.
	slices.SortStableFunc(locks, func(a, b *lock) int {
		if c := a.Created.Compare(b.Created); c != 0 {
			return c
		}
		return strings.Compare(a.file, b.file)
	})
	return locks
}

// Spare names a folder of this computer to keep the keys' files in a
// second time, and puts back from it a key the vault's folder lost: with
// its file gone, what a key sealed is gone.
func (v *Vault) Spare(dir string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.spare = dir
	v.restore()
}

// restore puts back the keys the folder lost and keeps those it has.
func (v *Vault) restore() {
	if v.spare == "" {
		return
	}
	for _, s := range readLocks(v.spare) {
		if !slices.ContainsFunc(v.locks, func(l *lock) bool { return l.ID == s.ID }) {
			if v.write(v.Dir, s) == nil {
				v.locks = append(v.locks, s)
			}
		}
	}
	for _, l := range v.locks {
		data, err := json.MarshalIndent(l, "", "  ")
		if err != nil {
			continue
		}
		if old, _ := os.ReadFile(filepath.Join(v.spare, l.file)); string(old) != string(data) {
			_ = v.write(v.spare, l)
		}
	}
}

func (v *Vault) write(dir string, l *lock) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(dir, l.file)
	if err := os.WriteFile(p+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// current is the key that seals: the oldest not turned off.
func (v *Vault) current() *lock {
	for _, l := range v.locks {
		if !l.Off {
			return l
		}
	}
	return nil
}

// On tells whether the vault seals: a second password was set, and not
// turned off.
func (v *Vault) On() bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.current() != nil
}

// Known tells whether there is a key at all, one turned off too: what it
// sealed may still be about.
func (v *Vault) Known() bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.locks) > 0
}

// Unlocked tells whether a key is open.
func (v *Vault) Unlocked() bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.keys) > 0
}

// Changed is when the password was last set.
func (v *Vault) Changed() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	if l := v.current(); l != nil {
		return l.Changed
	}
	return time.Time{}
}

func newID() string {
	const hex = "0123456789abcdef"
	var b [3]byte
	_, _ = rand.Read(b[:])
	return string([]byte{hex[b[0]>>4], hex[b[0]&15], hex[b[1]>>4], hex[b[1]&15], hex[b[2]>>4], hex[b[2]&15]})
}

// Setup makes a key locked by password, and returns the recovery code,
// which is shown once and kept nowhere. The vault is left unlocked.
func (v *Vault) Setup(password string, now time.Time) (code string, err error) {
	if len([]rune(password)) < MinPassword {
		return "", ErrShort
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.current() != nil {
		return "", errors.New("已经设置过二次密码了，可能是在另一台电脑上：用那个密码解锁")
	}
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	l := &lock{Version: 1, ID: newID(), Public: pub[:], Created: now, Changed: now}
	l.file = l.ID + ".json"
	if code, err = l.lockWith(priv, password); err != nil {
		return "", err
	}
	if err := v.write(v.Dir, l); err != nil {
		return "", err
	}
	v.locks = append(v.locks, l)
	v.hold(l.ID, priv)
	v.restore()
	return code, nil
}

// lockWith locks the private key by a password and by a new recovery
// code, which it returns.
func (l *lock) lockWith(priv *[32]byte, password string) (code string, err error) {
	code = newCode()
	if l.Password, err = wrapKey(priv, password); err != nil {
		return "", err
	}
	if l.Recovery, err = wrapKey(priv, cleanCode(code)); err != nil {
		return "", err
	}
	return code, nil
}

func wrapKey(priv *[32]byte, secret string) (wrap, error) {
	w := wrap{KDF: "argon2id", Salt: make([]byte, 16), kdf: cost}
	if _, err := rand.Read(w.Salt); err != nil {
		return w, err
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return w, err
	}
	key := w.key(secret)
	w.Box = secretbox.Seal(nonce[:], priv[:], &nonce, &key)
	return w, nil
}

func (w wrap) key(secret string) (key [32]byte) {
	copy(key[:], argon2.IDKey([]byte(secret), w.Salt, w.Time, w.Memory, w.Threads, 32))
	return key
}

// open is the private key, when secret is what locked it.
func (w wrap) open(secret string) (*[32]byte, bool) {
	if w.KDF != "argon2id" || len(w.Box) < 24 || w.Time == 0 || w.Memory == 0 || w.Threads == 0 ||
		w.Memory > 4*1024*1024 || w.Time > 64 { // a file is not to ask for all the memory there is
		return nil, false
	}
	var nonce [24]byte
	copy(nonce[:], w.Box)
	key := w.key(secret)
	out, ok := secretbox.Open(nil, w.Box[24:], &nonce, &key)
	if !ok || len(out) != 32 {
		return nil, false
	}
	var priv [32]byte
	copy(priv[:], out)
	return &priv, true
}

func (v *Vault) hold(id string, priv *[32]byte) {
	if v.keys == nil {
		v.keys, v.memo = map[string]*[32]byte{}, map[string]string{}
	}
	v.keys[id] = priv
}

// Unlock opens the keys the password locks.
func (v *Vault) Unlock(password string) error {
	return v.unlock(password, func(l *lock) wrap { return l.Password }, ErrWrong)
}

// UnlockCode opens the keys a recovery code locks.
func (v *Vault) UnlockCode(code string) error {
	return v.unlock(cleanCode(code), func(l *lock) wrap { return l.Recovery }, ErrCode)
}

func (v *Vault) unlock(secret string, of func(*lock) wrap, wrong error) error {
	// The keys' wraps are taken under the lock and tried outside it:
	// Argon2 takes a while, and the window asks at every frame whether
	// the vault is unlocked, so that held through it, an unlock off the
	// window's thread held the window all the same.
	v.mu.Lock()
	ids, wraps := make([]string, len(v.locks)), make([]wrap, len(v.locks))
	for i, l := range v.locks {
		ids[i], wraps[i] = l.ID, of(l)
	}
	locked := v.locked
	v.mu.Unlock()
	if len(ids) == 0 {
		return ErrOff
	}
	opened := map[string]*[32]byte{}
	for i, w := range wraps {
		if priv, ok := w.open(secret); ok {
			opened[ids[i]] = priv
		}
	}
	if len(opened) == 0 {
		return wrong
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.locked != locked { // locked meanwhile: that was asked last
		return ErrLocked
	}
	for id, priv := range opened {
		v.hold(id, priv)
	}
	return nil
}

// Lock forgets the keys opened.
func (v *Vault) Lock() {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, k := range v.keys {
		*k = [32]byte{}
	}
	v.keys, v.memo = nil, nil
	v.locked++
}

// Change locks the keys now open by a new password and a new recovery
// code, which it returns: the old password and the old code open nothing
// more. What was sealed stays as it is.
func (v *Vault) Change(password string, now time.Time) (code string, err error) {
	if len([]rune(password)) < MinPassword {
		return "", ErrShort
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.keys) == 0 {
		return "", ErrLocked
	}
	code = newCode()
	var done []*lock
	for _, l := range v.locks {
		priv := v.keys[l.ID]
		if priv == nil {
			continue
		}
		n := *l
		if n.Password, err = wrapKey(priv, password); err != nil {
			return "", err
		}
		if n.Recovery, err = wrapKey(priv, cleanCode(code)); err != nil {
			return "", err
		}
		n.Changed = now
		done = append(done, &n)
	}
	for _, n := range done {
		if err := v.write(v.Dir, n); err != nil {
			return "", err
		}
		for i, l := range v.locks {
			if l.file == n.file {
				v.locks[i] = n
			}
		}
	}
	v.restore()
	return code, nil
}

// TurnOff marks the keys as sealing nothing more. They still open what
// they sealed.
func (v *Vault) TurnOff(now time.Time) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, l := range v.locks {
		if l.Off {
			continue
		}
		l.Off, l.Changed = true, now
		if err := v.write(v.Dir, l); err != nil {
			return err
		}
	}
	v.restore()
	return nil
}

// Seal is a secret as a token, by the key that seals; ok is false when
// the vault is not on. It asks for no password.
func (v *Vault) Seal(plain string) (token string, ok bool) {
	if v == nil || plain == "" {
		return "", false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	l := v.current()
	if l == nil {
		return "", false
	}
	if t, ok := v.memo[l.ID+"\x00"+plain]; ok {
		return t, true
	}
	var pub [32]byte
	copy(pub[:], l.Public)
	out, err := box.SealAnonymous(nil, []byte(plain), &pub, rand.Reader)
	if err != nil {
		return "", false
	}
	token = tokenStart + l.ID + "." + base64.RawURLEncoding.EncodeToString(out) + tokenEnd
	if v.memo != nil {
		v.memo[l.ID+"\x00"+plain] = token
	}
	return token, true
}

// open is what a token seals, when its key is open.
func (v *Vault) open(token string) (string, bool) {
	body, ok := strings.CutPrefix(token, tokenStart)
	if !ok {
		return "", false
	}
	if body, ok = strings.CutSuffix(body, tokenEnd); !ok {
		return "", false
	}
	id, data, ok := strings.Cut(body, ".")
	if !ok {
		return "", false
	}
	priv := v.keys[id]
	if priv == nil {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return "", false
	}
	for _, l := range v.locks {
		if l.ID != id {
			continue
		}
		var pub [32]byte
		copy(pub[:], l.Public)
		if out, ok := box.OpenAnonymous(nil, raw, &pub, priv); ok {
			v.memo[id+"\x00"+string(out)] = token
			return string(out), true
		}
	}
	return "", false
}

// Opened is what one token seals, opened once: a text sealed whole may
// hold tokens sealed before it, which are left as they are, for who is to
// know where each secret was (Reveal opens them all).
func (v *Vault) Opened(token string) (plain string, ok bool) {
	if v == nil {
		return "", false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.open(token)
}

// Reveal is s with the secrets sealed in it opened; ok is false when one
// could not be, which is left as it is.
func (v *Vault) Reveal(s string) (plain string, ok bool) {
	if !Has(s) {
		return s, true
	}
	if v == nil {
		return s, false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	ok = true
	// A text sealed whole may hold what was sealed before it: opened in turn.
	for range 4 {
		again := false
		s = Token.ReplaceAllStringFunc(s, func(t string) string {
			p, opened := v.open(t)
			if !opened {
				ok = false
				return t
			}
			again = again || Has(p)
			return p
		})
		if !again {
			break
		}
	}
	return s, ok
}

// The recovery code: 24 characters of Crockford's alphabet (no I, L, O
// or U), 120 bits, written in fours.
const codeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newCode() string {
	var raw [24]byte
	_, _ = rand.Read(raw[:])
	var b strings.Builder
	for i, r := range raw {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(codeAlphabet[r&31])
	}
	return b.String()
}

// cleanCode is a code as it was made, however it was written down: in
// small letters, with spaces, an O for a 0 or an I or L for a 1.
func cleanCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch r {
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		if strings.ContainsRune(codeAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsCode tells whether s is written as a recovery code is.
func IsCode(s string) bool { return len(cleanCode(s)) == 24 }

// Quick makes the passwords' work light, for tests that set many.
func Quick() { cost = kdf{Time: 1, Memory: 64, Threads: 1} }
