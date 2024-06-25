package transform

// String transforms string value addressed by the path.
func String(obj []byte, path string, transformFn func(field string) string) ([]byte, error) {
	pathStr := GetBytes(obj, path)
	if !pathStr.Exists() {
		return obj, nil
	}
	rwrString := transformFn(pathStr.String())
	return SetBytes(obj, path, rwrString)
}

// Object transforms object value addressed by the path.
func Object(obj []byte, path string, transformFn func(item []byte) ([]byte, error)) ([]byte, error) {
	pathObj := GetBytes(obj, path)
	if !pathObj.IsObject() {
		return obj, nil
	}
	rwrObj, err := transformFn([]byte(pathObj.Raw))
	if err != nil {
		return nil, err
	}
	return SetRawBytes(obj, path, rwrObj)
}

// MapStringString transforms map[string]string value addressed by path.
func MapStringString(obj []byte, mapPath string, transformFn func(k, v string) (string, string)) ([]byte, error) {
	m := GetBytes(obj, mapPath).Map()
	if len(m) == 0 {
		return obj, nil
	}
	newMap := make(map[string]string, len(m))
	for k, v := range m {
		newK, newV := transformFn(k, v.String())
		newMap[newK] = newV
	}

	return SetBytes(obj, mapPath, newMap)
}

// Array gets array by the path and transforms each item using transformFn.
// Use Root path to transform object itself.
func Array(obj []byte, arrayPath string, transformFn func(item []byte) ([]byte, error)) ([]byte, error) {
	// Transform each item in list. Put back original items if transformFn returns nil bytes.
	items := GetBytes(obj, arrayPath).Array()
	if len(items) == 0 {
		return obj, nil
	}
	rwrItems := []byte(`[]`)
	for _, item := range items {
		rwrItem, err := transformFn([]byte(item.Raw))
		if err != nil {
			return nil, err
		}
		// Put original item back.
		if rwrItem == nil {
			rwrItem = []byte(item.Raw)
		}
		rwrItems, err = SetRawBytes(rwrItems, "-1", rwrItem)
		if err != nil {
			return nil, err
		}
	}

	return SetRawBytes(obj, arrayPath, rwrItems)
}

// ArrayOfStrings transforms array of strings addressed by the path.
func ArrayOfStrings(obj []byte, arrayPath string, transformFn func(item string) string) ([]byte, error) {
	// Transform each item in list. Put back original items if transformFn returns nil bytes.
	items := GetBytes(obj, arrayPath).Array()
	if len(items) == 0 {
		return obj, nil
	}
	rwrItems := make([]string, len(items))
	for i, item := range items {
		rwrItems[i] = transformFn(item.String())
	}

	return SetBytes(obj, arrayPath, rwrItems)
}

func Apply(obj []byte, transformFns ...func(obj []byte) ([]byte, error)) ([]byte, error) {
	var err error
	for _, fn := range transformFns {
		obj, err = fn(obj)
		if err != nil {
			return nil, err
		}
	}
	return obj, nil
}
