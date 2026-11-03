// 日本語のコメントがあっても、エラーの位置はずれない。
package unsupported

type Result struct {
	Big int64 `json:"big"`
}

func Compute(events chan int) Result {
	return Result{}
}
