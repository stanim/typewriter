package packages

type snippet struct {
	imports []string
	source  string
}

var snippets = map[string]snippet{
	"i32": snippet{source: `
// i32 converts float32 constants to int
// See https://groups.google.com/d/msg/golang-nuts/MntI1N_tAlA/CUKflVJeer8J
func i32(x float32) int {
	return int(x)
}`},
	"i64": snippet{source: `
// i64 converts float64 constants to int
// See https://groups.google.com/d/msg/golang-nuts/MntI1N_tAlA/CUKflVJeer8J
func i64(x float64) int {
	return int(x)
}`},
}
