package daemon

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	market "github.com/tutti-os/tutti/packages/connector/host"
)

const (
	connectorSignatureAlgorithm = "Ed25519-SHA256"
	releaseSignatureContext     = "tutti.connector.release.v1\x00"
	catalogSignatureContext     = "tutti.connector.catalog.v1\x00"
	connectorArtifactRealm      = "tutti.connector.artifacts.v1"
)

type catalogTrustVerifier struct {
	keyringVersion uint64
	keys           map[string]ed25519.PublicKey
	now            func() time.Time
}

type signedCatalogPayload struct {
	Sequence     uint64             `json:"sequence"`
	IssuedAt     time.Time          `json:"issuedAt"`
	ExpiresAt    time.Time          `json:"expiresAt"`
	NextUpdateAt time.Time          `json:"nextUpdateAt"`
	Catalog      signedCatalogIndex `json:"catalog"`
}

type signedCatalogIndex struct {
	SchemaVersion string                       `json:"schemaVersion"`
	Releases      []signedCatalogReleaseStatus `json:"releases"`
}

type signedCatalogReleaseStatus struct {
	ConnectorKey          string   `json:"connectorKey"`
	ReleaseDigest         string   `json:"releaseDigest"`
	Version               string   `json:"version"`
	Status                string   `json:"status"`
	PublishedMarkets      []string `json:"publishedMarkets"`
	ManifestSHA256        string   `json:"manifestSha256"`
	ArtifactSHA256        string   `json:"artifactSha256"`
	ArtifactObjectVersion string   `json:"artifactObjectVersion"`
	EnvelopeSHA256        string   `json:"envelopeSha256"`
	SignatureKeyID        string   `json:"signatureKeyId"`
	Signature             string   `json:"signature"`
}

func newCatalogTrustVerifier(keyringVersion uint64, keys map[string]ed25519.PublicKey) (*catalogTrustVerifier, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if keyringVersion == 0 {
		return nil, errors.New("connector market signing keyring version is required")
	}
	cloned := make(map[string]ed25519.PublicKey, len(keys))
	for keyID, key := range keys {
		if strings.TrimSpace(keyID) == "" || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("connector market trust root is invalid")
		}
		cloned[keyID] = append(ed25519.PublicKey(nil), key...)
	}
	return &catalogTrustVerifier{keyringVersion: keyringVersion, keys: cloned, now: time.Now}, nil
}

func (source *CatalogSource) verifyProjection(ctx context.Context, projection wireConnectorCatalogResponse) (market.CatalogTrustState, error) {
	if source == nil || source.trustVerifier == nil {
		return market.CatalogTrustState{}, errors.New("connector market trust roots are unavailable")
	}
	source.trustMu.Lock()
	defer source.trustMu.Unlock()
	previous, exists, loadErr := source.trustStateStore.LoadCatalogTrustState(ctx, source.expectedMarketType)
	if loadErr != nil {
		return market.CatalogTrustState{}, fmt.Errorf("load connector catalog trust state: %w", loadErr)
	}
	if !exists {
		previous = market.CatalogTrustState{}
	}
	catalog, next, err := source.trustVerifier.verifyCatalog(projection.Snapshot.SignedSnapshot, previous)
	if err != nil {
		return market.CatalogTrustState{}, err
	}
	statusByRelease := make(map[string]signedCatalogReleaseStatus, len(catalog.Catalog.Releases))
	required := make(map[string]struct{})
	for _, status := range catalog.Catalog.Releases {
		key := status.ConnectorKey + "\x00" + status.ReleaseDigest
		statusByRelease[key] = status
		if status.Status == "available" && containsString(status.PublishedMarkets, projection.MarketType) {
			required[key] = struct{}{}
		}
	}
	for _, release := range projection.Releases {
		key := release.ConnectorKey + "\x00" + release.ReleaseDigest
		status, exists := statusByRelease[key]
		if !exists || status.Status != "available" || !containsString(status.PublishedMarkets, projection.MarketType) {
			return market.CatalogTrustState{}, errors.New("connector release is not active in the signed catalog")
		}
		payload, digest, verifyErr := source.trustVerifier.verifyRelease(release.SignedEnvelope)
		if verifyErr != nil {
			return market.CatalogTrustState{}, verifyErr
		}
		if release.Artifact == nil || digest != release.ReleaseDigest || status.EnvelopeSHA256 != digest ||
			payload.ItemKey != release.ConnectorKey || payload.Version != release.Version || status.Version != release.Version ||
			payload.ManifestSHA256 != release.Manifest.SHA256 || status.ManifestSHA256 != release.Manifest.SHA256 ||
			payload.ArtifactStorageRealm != connectorArtifactRealm || payload.ArtifactObjectVersion != release.Artifact.ObjectVersion ||
			payload.ArtifactSHA256 != release.Artifact.SHA256 || status.ArtifactSHA256 != release.Artifact.SHA256 ||
			status.ArtifactObjectVersion != release.Artifact.ObjectVersion || payload.ArtifactSizeBytes != int64(release.Artifact.SizeBytes) ||
			payload.ArtifactMediaType != release.Artifact.MediaType || status.SignatureKeyID != release.SignedEnvelope.KeyID ||
			status.Signature != release.SignedEnvelope.Signature {
			return market.CatalogTrustState{}, errors.New("connector release projection does not match signed catalog and release payload")
		}
		delete(required, key)
	}
	if len(required) != 0 {
		return market.CatalogTrustState{}, errors.New("connector projection withheld an active signed release")
	}
	return next, nil
}

func (verifier *catalogTrustVerifier) verifyCatalog(document wireSignedDocument, previous market.CatalogTrustState) (signedCatalogPayload, market.CatalogTrustState, error) {
	if err := verifier.verifyDocument(catalogSignatureContext, document); err != nil {
		return signedCatalogPayload{}, market.CatalogTrustState{}, err
	}
	var payload signedCatalogPayload
	if err := decodeCanonicalDocument([]byte(document.CanonicalBytes), &payload); err != nil {
		return signedCatalogPayload{}, market.CatalogTrustState{}, fmt.Errorf("decode signed connector catalog: %w", err)
	}
	now := verifier.now().UTC()
	if payload.Sequence == 0 || payload.Catalog.SchemaVersion != "1" || payload.IssuedAt.IsZero() || payload.ExpiresAt.IsZero() ||
		payload.NextUpdateAt.IsZero() || !payload.IssuedAt.Before(payload.NextUpdateAt) || payload.NextUpdateAt.After(payload.ExpiresAt) ||
		payload.ExpiresAt.Sub(payload.IssuedAt) > 15*time.Minute || payload.IssuedAt.After(now.Add(30*time.Second)) || now.After(payload.ExpiresAt.Add(30*time.Second)) {
		return signedCatalogPayload{}, market.CatalogTrustState{}, errors.New("signed connector catalog freshness window is invalid")
	}
	if previous.Sequence > payload.Sequence || (previous.Sequence == payload.Sequence && previous.Sequence != 0 && previous.EnvelopeDigest != document.SHA256) {
		return signedCatalogPayload{}, market.CatalogTrustState{}, errors.New("signed connector catalog rollback or equivocation rejected")
	}
	if previous.KeyringVersion > verifier.keyringVersion {
		return signedCatalogPayload{}, market.CatalogTrustState{}, errors.New("connector market signing keyring rollback rejected")
	}
	if !previous.WallHighWater.IsZero() && now.Before(previous.WallHighWater.Add(-30*time.Second)) {
		return signedCatalogPayload{}, market.CatalogTrustState{}, errors.New("local clock rollback requires a newer trusted catalog")
	}
	if err := validateSignedCatalogOrder(payload.Catalog.Releases); err != nil {
		return signedCatalogPayload{}, market.CatalogTrustState{}, err
	}
	next := market.CatalogTrustState{KeyringVersion: verifier.keyringVersion, Sequence: payload.Sequence, EnvelopeDigest: document.SHA256,
		IssuedAt: payload.IssuedAt.UTC(), ExpiresAt: payload.ExpiresAt.UTC(), NextUpdateAt: payload.NextUpdateAt.UTC(),
		ObservedAt: now, WallHighWater: now}
	if previous.WallHighWater.After(next.WallHighWater) {
		next.WallHighWater = previous.WallHighWater
	}
	return payload, next, nil
}

func (verifier *catalogTrustVerifier) verifyRelease(document wireSignedDocument) (wireReleaseEnvelopePayload, string, error) {
	if err := verifier.verifyDocument(releaseSignatureContext, document); err != nil {
		return wireReleaseEnvelopePayload{}, "", err
	}
	var payload wireReleaseEnvelopePayload
	if err := decodeCanonicalDocument([]byte(document.CanonicalBytes), &payload); err != nil {
		return wireReleaseEnvelopePayload{}, "", fmt.Errorf("decode signed connector release: %w", err)
	}
	if payload.SchemaVersion != "1" || payload.ItemType != "connector" || payload.ItemKey == "" || payload.Version == "" ||
		payload.PublisherSubject == "" || payload.SourceRepository == "" || payload.CommitSHA == "" || payload.WorkflowRef == "" ||
		!isSHA256Hex(payload.ProvenanceDigest) || !isSHA256Hex(payload.ManifestSHA256) || !isSHA256Hex(payload.ArtifactSHA256) ||
		payload.ArtifactSizeBytes <= 0 || !safeArtifactKey(payload.ArtifactKey) || payload.ArtifactStorageRealm != connectorArtifactRealm ||
		payload.ArtifactObjectVersion == "" || payload.ArtifactMediaType == "" || payload.TrustTier == "" {
		return wireReleaseEnvelopePayload{}, "", errors.New("signed connector release payload is invalid")
	}
	return payload, document.SHA256, nil
}

func (verifier *catalogTrustVerifier) verifyDocument(domain string, document wireSignedDocument) error {
	if err := validateSignedDocumentDigest(document); err != nil || document.Algorithm != connectorSignatureAlgorithm {
		return errors.New("connector market signature envelope is invalid")
	}
	key, trusted := verifier.keys[document.KeyID]
	if !trusted {
		return errors.New("connector market signing key is not trusted")
	}
	signature, err := base64.StdEncoding.DecodeString(document.Signature)
	if err != nil {
		signature, err = base64.RawStdEncoding.DecodeString(document.Signature)
	}
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("connector market signature envelope is invalid")
	}
	message := append([]byte(domain), []byte(document.CanonicalBytes)...)
	digest := sha256.Sum256(message)
	if !ed25519.Verify(key, digest[:], signature) {
		return errors.New("connector market signature verification failed")
	}
	return nil
}

func decodeCanonicalDocument(data []byte, target any) error {
	if err := decodeStrictJSON(data, target); err != nil {
		return err
	}
	canonical, err := json.Marshal(target)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return errors.New("signed payload is not canonical JSON")
	}
	return nil
}

func validateSignedCatalogOrder(entries []signedCatalogReleaseStatus) error {
	keys := make([]string, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		key := entry.ConnectorKey + "\x00" + entry.ReleaseDigest
		if entry.ConnectorKey == "" || !isSHA256Hex(entry.ReleaseDigest) || entry.Version == "" ||
			(entry.Status != "available" && entry.Status != "superseded" && entry.Status != "security_revoked") ||
			entry.EnvelopeSHA256 != entry.ReleaseDigest || !isSHA256Hex(entry.ManifestSHA256) || !isSHA256Hex(entry.ArtifactSHA256) ||
			entry.ArtifactObjectVersion == "" || entry.SignatureKeyID == "" || entry.Signature == "" {
			return errors.New("signed connector catalog contains incomplete release evidence")
		}
		if _, duplicate := seen[key]; duplicate {
			return errors.New("signed connector catalog contains a duplicate release")
		}
		seen[key] = struct{}{}
		keys[index] = key
	}
	if !sort.StringsAreSorted(keys) {
		return errors.New("signed connector catalog releases must use canonical order")
	}
	return nil
}
