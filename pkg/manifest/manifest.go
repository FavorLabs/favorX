// Package manifest contains the abstractions needed for
// collection representation, It uses implementations
// in ethersphere/manifest repo under the hood.
package manifest

import (
	"context"
	"errors"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/file"
)

const DefaultManifestType = ManifestMantarayContentType

const (
	RootPath                      = "/"
	ReferenceLinkKey              = "reference"
	WebsiteIndexDocumentSuffixKey = "website-index-document"
	WebsiteErrorDocumentPathKey   = "website-error-document"
	EntryMetadataContentTypeKey   = "Content-Type"
	EntryMetadataDirnameKey       = "Dirname"
	EntryMetadataFilenameKey      = "Filename"
)

var (
	// ErrNotFound is returned when an Entry is not found in the manifest.
	ErrNotFound = errors.New("manifest: not found")

	// ErrInvalidManifestType is returned when an unknown manifest type
	// is provided to the function.
	ErrInvalidManifestType = errors.New("manifest: invalid type")

	// ErrMissingReference is returned when the reference for the manifest file
	// is missing.
	ErrMissingReference = errors.New("manifest: missing reference")
)

// StoreSizeFunc is a callback on every content size that will be stored by
// the Store function.
type StoreSizeFunc func(int64) error

// NodeIterFunc is a callback on each level.
type NodeIterFunc func(nodeType int, path, prefix, hash []byte, metadata map[string]string) error

// NodeType represents a Node stored file or directory
type NodeType int

const (
	File NodeType = iota
	Directory
	IndexItem
	Dirs
)

func (t NodeType) String() string {
	switch t {
	case File:
		return "file"
	case Directory:
		return "directory"
	case IndexItem:
		return "index"
	case Dirs:
		return "dirs"
	}
	return "unknown"
}

// Interface for operations with manifest.
type Interface interface {
	// Type returns manifest implementation type information
	Type() string
	// Add a manifest entry to the specified path.
	Add(context.Context, string, Entry) error
	// Remove a manifest entry on the specified path.
	Remove(context.Context, string) error
	// Copy a manifest entry to the specified path.
	Copy(context.Context, boson.Address, string, string, bool) error
	// Move a manifest entry to the specified path.
	Move(context.Context, boson.Address, string, string, bool) error
	// Lookup returns a manifest entry if one is found in the specified path.
	Lookup(context.Context, string) (Entry, error)
	// HasPrefix tests whether the specified prefix path exists.
	HasPrefix(context.Context, string) (bool, error)
	// Store stores the manifest, returning the resulting address.
	Store(context.Context, ...StoreSizeFunc) (boson.Address, error)
	// IterateDirectories is used to iterate over directory or file for the
	// manifest.
	IterateDirectories(context.Context, []byte, int, NodeIterFunc) error
	// IterateAddresses is used to iterate over chunks addresses for
	// the manifest.
	IterateAddresses(context.Context, boson.AddressIterFunc) error
}

// Entry represents a single manifest entry.
type Entry interface {
	// Reference returns the address of the file.
	Reference() boson.Address
	// Metadata returns the metadata of the file.
	Metadata() map[string]string
	Index() int64
}

// NewDefaultManifest creates a new manifest with default type.
func NewDefaultManifest(
	ls file.LoadSaver,
	encrypted bool,
) (Interface, error) {
	return NewManifest(DefaultManifestType, ls, encrypted)
}

// NewDefaultManifestReference creates a new manifest with default type.
func NewDefaultManifestReference(
	reference boson.Address,
	ls file.LoadSaver,
) (Interface, error) {
	return NewManifestReference(DefaultManifestType, reference, ls)
}

// NewManifest creates a new manifest.
func NewManifest(
	manifestType string,
	ls file.LoadSaver,
	encrypted bool,
) (Interface, error) {
	switch manifestType {
	case ManifestSimpleContentType:
		return NewSimpleManifest(ls)
	case ManifestMantarayContentType:
		return NewMantarayManifest(ls, encrypted)
	default:
		return nil, ErrInvalidManifestType
	}
}

// NewManifestReference loads existing manifest.
func NewManifestReference(
	manifestType string,
	reference boson.Address,
	ls file.LoadSaver,
) (Interface, error) {
	switch manifestType {
	case ManifestSimpleContentType:
		return NewSimpleManifestReference(reference, ls)
	case ManifestMantarayContentType:
		return NewMantarayManifestReference(reference, ls)
	default:
		return nil, ErrInvalidManifestType
	}
}

type manifestEntry struct {
	reference boson.Address
	metadata  map[string]string
	index     int64
	prefix    [][]byte
}

// NewEntry creates a new manifest entry.
func NewEntry(reference boson.Address, metadata map[string]string, index int64) Entry {
	return &manifestEntry{
		reference: reference,
		metadata:  metadata,
		index:     index,
	}
}

func (e *manifestEntry) Reference() boson.Address {
	return e.reference
}

func (e *manifestEntry) Metadata() map[string]string {
	return e.metadata
}

func (e *manifestEntry) Index() int64 {
	return e.index
}
