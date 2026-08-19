package sync

import (
	"time"

	"crumb/pkg/storage"
)

// Merge resolves base (the last state both sides agreed on), local (this
// machine's current secrets), and remote (the server's current secrets)
// into a single SecretStore using a three-way, per-key merge:
//
//   - a key unchanged from base on both sides keeps base's value
//   - a key changed on only one side takes that side's value
//   - a key changed on both sides (a true conflict) is resolved by
//     SecretEntry.Updated: whichever side's entry is newer wins. If exactly
//     one side deleted the key while the other edited it, the edit wins —
//     losing a secret silently is worse than a stale delete not taking
//     effect until the next sync.
//
// Returns the merged store along with the list of keys that hit a true
// same-key conflict, so callers can report them.
func Merge(base, local, remote storage.SecretStore) (storage.SecretStore, []string) {
	merged := make(storage.SecretStore)
	var conflicts []string

	keys := make(map[string]struct{}, len(base)+len(local)+len(remote))
	for k := range base {
		keys[k] = struct{}{}
	}
	for k := range local {
		keys[k] = struct{}{}
	}
	for k := range remote {
		keys[k] = struct{}{}
	}

	for k := range keys {
		b, bOK := base[k]
		l, lOK := local[k]
		r, rOK := remote[k]

		localChanged := !entryEqual(bOK, b, lOK, l)
		remoteChanged := !entryEqual(bOK, b, rOK, r)

		switch {
		case !localChanged && !remoteChanged:
			if bOK {
				merged[k] = b
			}
		case localChanged && !remoteChanged:
			if lOK {
				merged[k] = l
			}
		case !localChanged && remoteChanged:
			if rOK {
				merged[k] = r
			}
		default: // both sides changed this key relative to base: a true conflict
			conflicts = append(conflicts, k)
			switch {
			case lOK && !rOK:
				merged[k] = l // edit beats a concurrent delete
			case rOK && !lOK:
				merged[k] = r
			case lOK && rOK:
				if remoteIsNewerOrEqual(l.Updated, r.Updated) {
					merged[k] = r
				} else {
					merged[k] = l
				}
				// else: deleted on both sides - stays deleted
			}
		}
	}

	return merged, conflicts
}

func entryEqual(aOK bool, a storage.SecretEntry, bOK bool, b storage.SecretEntry) bool {
	if aOK != bOK {
		return false
	}
	if !aOK {
		return true
	}
	return a.Value == b.Value && a.Expires == b.Expires
}

// remoteIsNewerOrEqual reports whether remote's timestamp should win over
// local's in a same-key conflict. Remote wins on a tie, and whenever either
// timestamp fails to parse (remote already reflects a confirmed push, so it
// is the safer default).
func remoteIsNewerOrEqual(localUpdated, remoteUpdated string) bool {
	rt, err := time.Parse(time.RFC3339, remoteUpdated)
	if err != nil {
		return true
	}
	lt, err := time.Parse(time.RFC3339, localUpdated)
	if err != nil {
		return true
	}
	return !lt.After(rt)
}
