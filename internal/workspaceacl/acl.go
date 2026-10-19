// Package workspaceacl gives tenant directories POSIX ACLs for the tenants'
// tool UIDs (KI-96, ADR-018). The worker runs every tool process of a tenant
// as the tenant's tool UID, which is in no group: a tenant directory
// (<root>/<tenant>) grants it search access, and its default ACL gives the
// projects created inside read, write and search access for the tool UID
// and for the workspace group (10010: the Go Core and the worker). Another
// tenant's tool UID gets nothing.
//
// The ACLs are written as the kernel's extended attributes
// (system.posix_acl_access / system.posix_acl_default, linux/posix_acl_xattr.h)
// on a descriptor, without libacl or setfacl. The worker has the same codec
// (workers/codeforge/posix_acl.py); both are tested against
// testdata/acl_vectors.json.
package workspaceacl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

// Tags of ACL entries.
const (
	TagUserObj  uint16 = 0x01
	TagUser     uint16 = 0x02
	TagGroupObj uint16 = 0x04
	TagGroup    uint16 = 0x08
	TagMask     uint16 = 0x10
	TagOther    uint16 = 0x20
)

// UndefinedID is the ID of entries that name no user or group.
const UndefinedID uint32 = 0xFFFFFFFF

// Extended attribute names.
const (
	XattrAccess  = "system.posix_acl_access"
	XattrDefault = "system.posix_acl_default"
)

const xattrVersion = 2

// WorkspaceGID is the workspace group of the Go Core and the worker.
const WorkspaceGID = 10010

// ErrUnsupported is returned where POSIX ACLs cannot be set (not Linux).
var ErrUnsupported = errors.New("POSIX ACLs are not supported on this platform")

// Entry is one ACL entry: its tag, its permissions (r=4, w=2, x=1) and, for
// named users and groups, their ID.
type Entry struct {
	Tag  uint16
	Perm uint16
	ID   uint32
}

func obj(tag, perm uint16) Entry           { return Entry{Tag: tag, Perm: perm, ID: UndefinedID} }
func named(tag, perm uint16, id int) Entry { return Entry{Tag: tag, Perm: perm, ID: uint32(id)} } //nolint:gosec // G115: UIDs/GIDs are small positive numbers

// TenantAccess is the access ACL of a tenant directory: the tool UID may
// only search it (reach its projects, never list, create, rename or delete
// at tenant level).
func TenantAccess(toolUID int) []Entry {
	return []Entry{obj(TagUserObj, 7), named(TagUser, 1, toolUID), obj(TagGroupObj, 7), obj(TagMask, 7), obj(TagOther, 0)}
}

// TenantDefault is the default ACL of a tenant directory, which its
// projects inherit: read, write and search for the tool UID and the
// workspace group.
func TenantDefault(toolUID, workspaceGID int) []Entry {
	return []Entry{
		obj(TagUserObj, 7), named(TagUser, 7, toolUID), obj(TagGroupObj, 7),
		named(TagGroup, 7, workspaceGID), obj(TagMask, 7), obj(TagOther, 0),
	}
}

// ProjectAccess is the access ACL of a project directory (what a project
// inherits; also set on an adopted workspace): the same as TenantDefault.
func ProjectAccess(toolUID, workspaceGID int) []Entry {
	return TenantDefault(toolUID, workspaceGID)
}

// sorted returns entries in the order the kernel requires: by tag, then ID.
func sorted(entries []Entry) []Entry {
	out := slices.Clone(entries)
	slices.SortFunc(out, func(a, b Entry) int {
		if a.Tag != b.Tag {
			return int(a.Tag) - int(b.Tag)
		}
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	return out
}

// Encode returns the extended attribute value of an ACL.
func Encode(entries []Entry) []byte {
	out := make([]byte, 4, 4+8*len(entries))
	binary.LittleEndian.PutUint32(out, xattrVersion)
	for _, e := range sorted(entries) {
		out = binary.LittleEndian.AppendUint16(out, e.Tag)
		out = binary.LittleEndian.AppendUint16(out, e.Perm)
		out = binary.LittleEndian.AppendUint32(out, e.ID)
	}
	return out
}

// Decode parses an extended attribute value.
func Decode(data []byte) ([]Entry, error) {
	if len(data) < 4 || (len(data)-4)%8 != 0 {
		return nil, fmt.Errorf("posix acl: %d bytes is no ACL", len(data))
	}
	if v := binary.LittleEndian.Uint32(data); v != xattrVersion {
		return nil, fmt.Errorf("posix acl: version %d, want %d", v, xattrVersion)
	}
	entries := make([]Entry, 0, (len(data)-4)/8)
	for off := 4; off < len(data); off += 8 {
		entries = append(entries, Entry{
			Tag:  binary.LittleEndian.Uint16(data[off:]),
			Perm: binary.LittleEndian.Uint16(data[off+2:]),
			ID:   binary.LittleEndian.Uint32(data[off+4:]),
		})
	}
	return entries, nil
}

// Equal reports whether two ACLs have the same entries, in any order.
func Equal(a, b []Entry) bool {
	return slices.Equal(sorted(a), sorted(b))
}
