package model

func AllModels() []any {
	return []any{
		&UserIdentity{},
		&Tag{},
		&PostTag{},
		&Post{},
		&Comment{},
		&Like{},
		&Favorite{},
		&SensitiveWord{},
		&ReviewQueue{},
	}
}
