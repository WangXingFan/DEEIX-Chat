package conversation

func getTraceString(value any) string {
	text, _ := value.(string)
	return text
}

func traceInt64(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}
