package db

// BlockListSource represents the source of a blocked domain
type BlockListSource string

const (
	// SourceStevenBlack - domains from StevenBlack blocklist
	SourceStevenBlack BlockListSource = "StevenBlack"

	// SourceEasyList - domains from EasyList blocklist
	SourceEasyList BlockListSource = "EasyList"

	// SourceSuggestedToBlock - domains suggested by users to block
	SourceSuggestedToBlock BlockListSource = "suggestedToBlock"
)

// String returns the string representation of the source
func (s BlockListSource) String() string {
	return string(s)
}
