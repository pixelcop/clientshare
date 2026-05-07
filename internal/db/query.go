package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var instance *gorm.DB

func SetDB(db *gorm.DB) {
	instance = db
}

func G[T any](opts ...clause.Expression) gorm.Interface[T] {
	return gorm.G[T](instance, opts...)
}
