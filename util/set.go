package util

// RemoveDuplicates 去除字符串数组中的重复元素
func RemoveDuplicates(strings []string) []string {
	encountered := map[string]bool{}
	result := []string{}

	for _, str := range strings {
		if !encountered[str] {
			encountered[str] = true
			result = append(result, str)
		}
	}

	return result
}
