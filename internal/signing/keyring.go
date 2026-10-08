// Package signing holds each project's ed25519 artifact signing keys. A model
// variant is signed when it is finalized (after its sha256 is verified), and
// devices verify the signature before they activate the model, so a model
// file or hash changed after publishing is rejected on the device.
package signing

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fleetml/fleetml/internal/artifact"
)

// MinMasterKeyLength is the shortest SIGNING_MASTER_KEY accepted.
const MinMasterKeyLength = 32

// Key statuses.
const (
	StatusActive   = "active"   // signs new artifacts
	StatusRetiring = "retiring" // rotated out; accepted until NotAfter
)

// ErrNotFound is returned for an unknown variant or a project without a key.
var ErrNotFound = errors.New("not found")

// Key is a project's public signing key.
type Key struct {
	KeyID     string     `json:"key_id"`
	ProjectID string     `json:"project_id"`
	PublicKey []byte     `json:"public_key"` // 32 bytes; base64 in JSON
	Status    string     `json:"status"`
	NotAfter  *time.Time `json:"not_after,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Signature is a variant's signature and the key that made it.
type Signature struct {
	KeyID     string
	Signature []byte
}

// Keyring creates, rotates and uses the signing keys.
type Keyring struct {
	pool *pgxpool.Pool
	aead cipher.AEAD
}

// NewKeyring derives the key-encryption key from masterKey (HKDF-SHA256).
func NewKeyring(pool *pgxpool.Pool, masterKey string) (*Keyring, error) {
	if len(masterKey) < MinMasterKeyLength {
		return nil, fmt.Errorf("SIGNING_MASTER_KEY must be at least %d characters", MinMasterKeyLength)
	}
	kek, err := hkdf.Key(sha256.New, []byte(masterKey), []byte("fleetml-signing"), "private-key-encryption-v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Keyring{pool: pool, aead: aead}, nil
}

// seal encrypts a private key's seed, bound to its key ID.
func (k *Keyring) seal(seed []byte, keyID string) ([]byte, error) {
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return k.aead.Seal(nonce, nonce, seed, []byte(keyID)), nil
}

func (k *Keyring) open(enc []byte, keyID string) (ed25519.PrivateKey, error) {
	n := k.aead.NonceSize()
	if len(enc) < n {
		return nil, errors.New("signing key: ciphertext too short")
	}
	seed, err := k.aead.Open(nil, enc[:n], enc[n:], []byte(keyID))
	if err != nil {
		return nil, fmt.Errorf("signing key %s: decryption failed (was SIGNING_MASTER_KEY changed?): %w", keyID, err)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// querier is satisfied by *pgxpool.Pool and pgx.Tx.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// createKey stores a new active key for the project.
func (k *Keyring) createKey(ctx context.Context, q querier, projectID string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	id := artifact.KeyID(pub)
	enc, err := k.seal(priv.Seed(), id)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx,
		`INSERT INTO signing_keys (project_id, key_id, public_key, private_key_enc, status) VALUES ($1, $2, $3, $4, 'active')
		 ON CONFLICT (project_id) WHERE status = 'active' DO NOTHING`, projectID, id, []byte(pub), enc)
	if err != nil {
		return fmt.Errorf("creating signing key: %w", err)
	}
	return nil
}

// activeSigner returns the project's active key ID and private key, creating
// the key on first use.
func (k *Keyring) activeSigner(ctx context.Context, q querier, projectID string) (string, ed25519.PrivateKey, error) {
	for attempt := 0; attempt < 2; attempt++ {
		var id string
		var enc []byte
		err := q.QueryRow(ctx, `SELECT key_id, private_key_enc FROM signing_keys WHERE project_id = $1 AND status = 'active'`, projectID).Scan(&id, &enc)
		if errors.Is(err, pgx.ErrNoRows) && attempt == 0 {
			if err := k.createKey(ctx, q, projectID); err != nil {
				return "", nil, err
			}
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("loading signing key: %w", err)
		}
		priv, err := k.open(enc, id)
		return id, priv, err
	}
	return "", nil, fmt.Errorf("loading signing key: %w", ErrNotFound)
}

// EnsureKey makes sure the project has an active key and returns its keys.
func (k *Keyring) EnsureKey(ctx context.Context, projectID string) ([]Key, error) {
	if _, _, err := k.activeSigner(ctx, k.pool, projectID); err != nil {
		return nil, err
	}
	return k.Keys(ctx, projectID)
}

const variantForSigning = `SELECT m.project_id::text, ver.model_id::text, ver.version, mv.blob_sha256
	FROM model_variants mv JOIN model_versions ver ON ver.id = mv.version_id JOIN models m ON m.id = ver.model_id
	WHERE mv.id = $1`

// SignVariant signs a variant with its project's active key and stores the signature.
func (k *Keyring) SignVariant(ctx context.Context, variantID string) error {
	var q querier = k.pool
	var projectID, modelID, version, sha string
	err := q.QueryRow(ctx, variantForSigning, variantID).Scan(&projectID, &modelID, &version, &sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("signing variant %s: %w", variantID, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("signing variant %s: %w", variantID, err)
	}
	keyID, priv, err := k.activeSigner(ctx, q, projectID)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE model_variants SET signature = $2, signing_key_id = $3 WHERE id = $1`,
		variantID, ed25519.Sign(priv, artifact.Message(modelID, version, sha)), keyID)
	return err
}

const keyColumns = `key_id, project_id::text, public_key, status, not_after, created_at`

// accepted selects the keys devices accept: the active one and retiring ones
// still in their grace period.
const accepted = `(status = 'active' OR (status = 'retiring' AND not_after > NOW()))`

func scanKeys(rows pgx.Rows) ([]Key, error) {
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		var key Key
		if err := rows.Scan(&key.KeyID, &key.ProjectID, &key.PublicKey, &key.Status, &key.NotAfter, &key.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// Keys returns the project's keys that devices accept, the active key first.
func (k *Keyring) Keys(ctx context.Context, projectID string) ([]Key, error) {
	rows, err := k.pool.Query(ctx, `SELECT `+keyColumns+` FROM signing_keys WHERE project_id = $1 AND `+accepted+
		` ORDER BY status = 'active' DESC, created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing signing keys: %w", err)
	}
	return scanKeys(rows)
}

// DeviceKeys returns the keys of the device's project.
func (k *Keyring) DeviceKeys(ctx context.Context, deviceID string) ([]Key, error) {
	rows, err := k.pool.Query(ctx, `SELECT `+keyColumns+` FROM signing_keys WHERE project_id = (SELECT project_id FROM devices WHERE id = $1) AND `+accepted+
		` ORDER BY status = 'active' DESC, created_at DESC`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("listing device signing keys: %w", err)
	}
	return scanKeys(rows)
}

// VariantSignatures returns the stored signatures of the given variants.
func (k *Keyring) VariantSignatures(ctx context.Context, variantIDs []string) (map[string]Signature, error) {
	out := map[string]Signature{}
	if len(variantIDs) == 0 {
		return out, nil
	}
	rows, err := k.pool.Query(ctx, `SELECT id::text, signing_key_id, signature FROM model_variants WHERE id = ANY($1::uuid[]) AND signature IS NOT NULL`, variantIDs)
	if err != nil {
		return nil, fmt.Errorf("loading variant signatures: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s Signature
		if err := rows.Scan(&id, &s.KeyID, &s.Signature); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

// Rotate makes a new active key. The previous key keeps being accepted for
// grace, and every READY variant whose signature verifies with it is signed
// again with the new key. It returns the new key's ID and how many variants
// were signed again.
func (k *Keyring) Rotate(ctx context.Context, projectID string, grace time.Duration) (newKeyID string, resigned int, err error) {
	tx, err := k.pool.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var q querier = tx

	// Serialize rotations of one project.
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('fleetml-signing:' || $1))`, projectID); err != nil {
		return "", 0, err
	}
	var oldID string
	var oldPub []byte
	err = q.QueryRow(ctx, `SELECT key_id, public_key FROM signing_keys WHERE project_id = $1 AND status = 'active'`, projectID).Scan(&oldID, &oldPub)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, err
	}
	if oldID != "" {
		if _, err := q.Exec(ctx, `UPDATE signing_keys SET status = 'retiring', not_after = $2 WHERE key_id = $1`, oldID, time.Now().Add(grace)); err != nil {
			return "", 0, err
		}
	}
	// Keys past their grace period are of no further use.
	if _, err := q.Exec(ctx, `DELETE FROM signing_keys WHERE project_id = $1 AND status = 'retiring' AND not_after < NOW()`, projectID); err != nil {
		return "", 0, err
	}
	if err := k.createKey(ctx, q, projectID); err != nil {
		return "", 0, err
	}
	newKeyID, priv, err := k.activeSigner(ctx, q, projectID)
	if err != nil {
		return "", 0, err
	}

	if oldID != "" {
		rows, err := q.Query(ctx, `SELECT mv.id::text, ver.model_id::text, ver.version, mv.blob_sha256, mv.signature
			FROM model_variants mv JOIN model_versions ver ON ver.id = mv.version_id JOIN models m ON m.id = ver.model_id
			WHERE m.project_id = $1 AND mv.status = 'READY' AND mv.signing_key_id = $2`, projectID, oldID)
		if err != nil {
			return "", 0, err
		}
		type resign struct {
			id  string
			sig []byte
		}
		var todo []resign
		for rows.Next() {
			var id, modelID, version, sha string
			var sig []byte
			if err := rows.Scan(&id, &modelID, &version, &sha, &sig); err != nil {
				rows.Close()
				return "", 0, err
			}
			msg := artifact.Message(modelID, version, sha)
			// Only what the old key vouched for is carried over.
			if ed25519.Verify(oldPub, msg, sig) {
				todo = append(todo, resign{id, ed25519.Sign(priv, msg)})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return "", 0, err
		}
		for _, r := range todo {
			if _, err := q.Exec(ctx, `UPDATE model_variants SET signature = $2, signing_key_id = $3 WHERE id = $1`, r.id, r.sig, newKeyID); err != nil {
				return "", 0, err
			}
		}
		resigned = len(todo)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	return newKeyID, resigned, nil
}

// Backfill signs READY variants that have no signature yet (published
// before signing existed). It returns how many it signed.
func (k *Keyring) Backfill(ctx context.Context) (int, error) {
	rows, err := k.pool.Query(ctx, `SELECT id::text FROM model_variants WHERE status = 'READY' AND signature IS NULL`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := k.SignVariant(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// ProjectExists reports whether the project exists.
func (k *Keyring) ProjectExists(ctx context.Context, projectID string) (bool, error) {
	var ok bool
	err := k.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&ok)
	return ok, err
}
