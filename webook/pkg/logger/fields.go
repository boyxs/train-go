package logger

func Error(err error) Field {
	return Field{
		Key: "error",
		Val: err,
	}
}

func String(key string, val string) Field {
	return Field{
		Key: key,
		Val: val,
	}
}

func Int64(key string, val int64) Field {
	return Field{
		Key: key,
		Val: val,
	}
}

func Int(key string, val int) Field {
	return Field{
		Key: key,
		Val: val,
	}
}

func Int32(key string, val int32) Field {
	return Field{
		Key: key,
		Val: val,
	}
}

func Uint64(key string, val uint64) Field {
	return Field{
		Key: key,
		Val: val,
	}
}

func Strings(key string, val []string) Field {
	return Field{
		Key: key,
		Val: val,
	}
}
