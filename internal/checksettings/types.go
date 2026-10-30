package checksettings

// DefaultMaxAgeDays is the stale window when check settings omit max age.
const DefaultMaxAgeDays = 180

// Policy is the check positive pay setting for an institution or party.
type Policy struct {
	// ReturnOnMismatch maps field mismatches to a return decision.
	ReturnOnMismatch bool
	// MaxAgeDays is days after the register issue date before a check is expired.
	// Zero or less means DefaultMaxAgeDays.
	MaxAgeDays int
}
