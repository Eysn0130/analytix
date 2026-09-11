package finalauthority

const privateNamedWriteTempSuffixBytes = 12

type securePrivateFile struct {
	Digest string
	Body   []byte
}

func privateNamedWriteTempName(candidate, target string) bool {
	prefix := "." + target + "-"
	const suffix = ".tmp"
	if len(candidate) != len(prefix)+privateNamedWriteTempSuffixBytes*2+len(suffix) ||
		len(candidate) <= len(prefix)+len(suffix) ||
		candidate[:len(prefix)] != prefix || candidate[len(candidate)-len(suffix):] != suffix {
		return false
	}
	for _, current := range candidate[len(prefix) : len(candidate)-len(suffix)] {
		if current < '0' || current > '9' {
			if current < 'a' || current > 'f' {
				return false
			}
		}
	}
	return true
}
