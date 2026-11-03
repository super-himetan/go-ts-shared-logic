package multi

var count int

// Bump adds one to the counter shared with y.go.
func Bump() int {
	count++
	return count
}

// Echo has a parameter name that TypeScript reserves.
func Echo(class string) string {
	return class
}
