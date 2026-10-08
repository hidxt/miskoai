package notices

import _ "embed"

// Text contains licenses and notices distributed with the native executable.
//go:embed notices.txt
var Text string
