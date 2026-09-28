package service

func discoverWindowStart(page, pageSize, sourcePageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if sourcePageSize < 1 {
		sourcePageSize = pageSize
	}
	offset := (page - 1) * pageSize
	return offset/sourcePageSize + 1, offset % sourcePageSize
}
