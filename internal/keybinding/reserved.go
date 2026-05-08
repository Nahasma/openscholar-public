package keybinding

// ReservedKeys returns the set of keys that cannot be rebound by users.
// These are core input/control keys that must maintain their behavior.
func ReservedKeys() []string {
	return []string{
		"ctrl+c",  // interrupt / quit
		"enter",   // submit input
		"esc",     // cancel / dismiss
	}
}

// IsReserved checks if a key combination is reserved and cannot be rebound.
func IsReserved(key string) bool {
	normalized := normalizeKey(key)
	for _, reserved := range ReservedKeys() {
		if normalizeKey(reserved) == normalized {
			return true
		}
	}
	return false
}
