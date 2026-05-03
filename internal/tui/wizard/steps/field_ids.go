package steps

// Field/label IDs that recur across step form definitions, validators, and
// the review summary. Keeping them in one place lets goconst's check go
// quiet and prevents one site drifting from another.
const (
	fieldHost        = "host"
	fieldDomain      = "domain"
	fieldGateway     = "gateway"
	fieldInterface   = "interface"
	fieldBridge      = "bridge"
	fieldDataStorage = "data storage"
)
