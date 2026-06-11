package cli

import _ "embed"

//go:embed default_schema.yml
var defaultSchema []byte

func DefaultSchemaBytes() []byte {
	return append([]byte(nil), defaultSchema...)
}
