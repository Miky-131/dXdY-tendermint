package cryptostore

import (
	"strings"

	crypto "github.com/tendermint/go-crypto"
	keys "github.com/tendermint/go-crypto/keys"
)

// Manager combines encyption and storage implementation to provide
// a full-featured key manager
type Manager struct {
	es    encryptedStorage
	codec keys.Codec
}

func New(coder Encoder, store keys.Storage, codec keys.Codec) Manager {
	return Manager{
		es: encryptedStorage{
			coder: coder,
			store: store,
		},
		codec: codec,
	}
}

// exists just to make sure we fulfill the Signer interface
func (s Manager) assertSigner() keys.Signer {
	return s
}

// exists just to make sure we fulfill the Manager interface
func (s Manager) assertKeyManager() keys.Manager {
	return s
}

// Create adds a new key to the storage engine, returning error if
// another key already stored under this name
//
// algo must be a supported go-crypto algorithm:
//
func (s Manager) Create(name, passphrase, algo string) (keys.Info, string, error) {
	gen, err := getGenerator(algo)
	if err != nil {
		return keys.Info{}, "", err
	}
	key := gen.Generate()
	err = s.es.Put(name, passphrase, key)
	if err != nil {
		return keys.Info{}, "", err
	}
	seed, err := s.codec.BytesToWords(key.Bytes())
	phrase := strings.Join(seed, " ")
	return info(name, key), phrase, err
}

func (s Manager) Recover(name, passphrase, seedphrase string) (keys.Info, error) {
	words := strings.Split(strings.TrimSpace(seedphrase), " ")
	data, err := s.codec.WordsToBytes(words)
	if err != nil {
		return keys.Info{}, err
	}

	key, err := crypto.PrivKeyFromBytes(data)
	if err != nil {
		return keys.Info{}, err
	}

	// d00d, it worked!  create the bugger....
	err = s.es.Put(name, passphrase, key)
	return info(name, key), err
}

// List loads the keys from the storage and enforces alphabetical order
func (s Manager) List() (keys.Infos, error) {
	res, err := s.es.List()
	res.Sort()
	return res, err
}

// Get returns the public information about one key
func (s Manager) Get(name string) (keys.Info, error) {
	_, info, err := s.es.store.Get(name)
	return info, err
}

// Sign will modify the Signable in order to attach a valid signature with
// this public key
//
// If no key for this name, or the passphrase doesn't match, returns an error
func (s Manager) Sign(name, passphrase string, tx keys.Signable) error {
	key, _, err := s.es.Get(name, passphrase)
	if err != nil {
		return err
	}
	sig := key.Sign(tx.SignBytes())
	pubkey := key.PubKey()
	return tx.Sign(pubkey, sig)
}

// Export decodes the private key with the current password, encodes
// it with a secure one-time password and generates a sequence that can be
// Imported by another Manager
//
// This is designed to copy from one device to another, or provide backups
// during version updates.
func (s Manager) Export(name, oldpass, transferpass string) ([]byte, error) {
	key, _, err := s.es.Get(name, oldpass)
	if err != nil {
		return nil, err
	}

	res, err := s.es.coder.Encrypt(key, transferpass)
	return res, err
}

// Import accepts bytes generated by Export along with the same transferpass
// If they are valid, it stores the password under the given name with the
// new passphrase.
func (s Manager) Import(name, newpass, transferpass string, data []byte) error {
	key, err := s.es.coder.Decrypt(data, transferpass)
	if err != nil {
		return err
	}

	return s.es.Put(name, newpass, key)
}

// Delete removes key forever, but we must present the
// proper passphrase before deleting it (for security)
func (s Manager) Delete(name, passphrase string) error {
	// verify we have the proper password before deleting
	_, _, err := s.es.Get(name, passphrase)
	if err != nil {
		return err
	}
	return s.es.Delete(name)
}

// Update changes the passphrase with which a already stored key is encoded.
//
// oldpass must be the current passphrase used for encoding, newpass will be
// the only valid passphrase from this time forward
func (s Manager) Update(name, oldpass, newpass string) error {
	key, _, err := s.es.Get(name, oldpass)
	if err != nil {
		return err
	}

	// we must delete first, as Putting over an existing name returns an error
	s.Delete(name, oldpass)

	return s.es.Put(name, newpass, key)
}
