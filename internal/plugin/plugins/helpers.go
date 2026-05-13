package plugins

// Shared helpers for extracting typed values from plugin config maps.

func stringOr(cfg map[string]interface{}, key, def string) string {
	if v, ok := cfg[key].(string); ok && v != "" {
		return v
	}

	return def
}

func boolVal(cfg map[string]interface{}, key string) bool {
	v, _ := cfg[key].(bool)
	return v
}

func floatVal(cfg map[string]interface{}, key string) float64 {
	v, _ := cfg[key].(float64)
	return v
}

func stringMap(cfg map[string]interface{}, key string) map[string]string {
	raw, _ := cfg[key].(map[string]interface{})
	out := make(map[string]string, len(raw))

	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}

	return out
}

func stringList(cfg map[string]interface{}, key string) []string {
	raw, _ := cfg[key].([]interface{})
	out := make([]string, 0, len(raw))

	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}

	return out
}
