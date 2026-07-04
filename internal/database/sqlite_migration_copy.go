package database

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func copyModelTables(src, target *gorm.DB, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	var copied int64
	for _, m := range model.AllModels() {
		table, err := modelTableName(src, m)
		if err != nil {
			return copied, err
		}
		primaryColumns, err := modelPrimaryColumns(src, m)
		if err != nil {
			return copied, fmt.Errorf("inspect model %T primary keys: %w", m, err)
		}
		exists, err := sqliteTableExists(src, table)
		if err != nil {
			return copied, err
		}
		if !exists {
			continue
		}
		var sourceCount int64
		if err := src.Raw("SELECT COUNT(1) FROM " + quoteIdent(table)).Scan(&sourceCount).Error; err != nil {
			return copied, fmt.Errorf("count sqlite table %s: %w", table, err)
		}
		if sourceCount == 0 {
			continue
		}
		var targetCount int64
		if err := target.Raw("SELECT COUNT(1) FROM " + quoteIdent(table)).Scan(&targetCount).Error; err != nil {
			return copied, fmt.Errorf("count target table %s: %w", table, err)
		}
		modelType := reflect.TypeOf(m)
		if modelType.Kind() != reflect.Ptr {
			return copied, fmt.Errorf("model %T is not a pointer", m)
		}
		sliceType := reflect.SliceOf(modelType.Elem())
		slicePtr := reflect.New(sliceType)
		if err := src.Unscoped().Find(slicePtr.Interface()).Error; err != nil {
			return copied, fmt.Errorf("read sqlite table %s: %w", table, err)
		}
		filtered := slicePtr.Elem()
		if targetCount > 0 {
			primaryKeySet, err := targetPrimaryKeySet(target, table, primaryColumns)
			if err != nil {
				return copied, err
			}
			filtered = filterRowsMissingInTarget(target, table, primaryColumns, filtered, primaryKeySet)
		}
		if filtered.Len() == 0 {
			continue
		}
		filteredPtr := reflect.New(filtered.Type())
		filteredPtr.Elem().Set(filtered)
		if err := target.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(filteredPtr.Interface(), batchSize).Error; err != nil {
			return copied, fmt.Errorf("copy sqlite table %s: %w", table, err)
		}
		copied += int64(filtered.Len())
	}
	return copied, nil
}

func modelPrimaryColumns(db *gorm.DB, m any) ([]string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return nil, err
	}
	var cols []string
	for _, field := range stmt.Schema.PrimaryFields {
		cols = append(cols, field.DBName)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("no primary key columns")
	}
	return cols, nil
}

func targetPrimaryKeySet(target *gorm.DB, table string, primaryColumns []string) (map[string]struct{}, error) {
	if len(primaryColumns) != 1 {
		return nil, nil
	}
	var values []string
	if err := target.Raw("SELECT " + quoteIdent(primaryColumns[0]) + " FROM " + quoteIdent(table)).Scan(&values).Error; err != nil {
		return nil, fmt.Errorf("read target primary keys for table %s: %w", table, err)
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set, nil
}

func filterRowsMissingInTarget(target *gorm.DB, table string, primaryColumns []string, rows reflect.Value, primaryKeySet map[string]struct{}) reflect.Value {
	if rows.Kind() != reflect.Slice || rows.Len() == 0 || len(primaryColumns) == 0 {
		return rows
	}
	out := reflect.MakeSlice(rows.Type(), 0, rows.Len())
	for i := 0; i < rows.Len(); i++ {
		row := rows.Index(i)
		keys, ok := rowPrimaryKeys(row, primaryColumns)
		if !ok {
			out = reflect.Append(out, row)
			continue
		}
		if primaryKeySet != nil {
			if _, exists := primaryKeySet[fmt.Sprint(keys[primaryColumns[0]])]; !exists {
				out = reflect.Append(out, row)
			}
			continue
		}
		if !targetHasPrimaryKey(target, table, keys) {
			out = reflect.Append(out, row)
		}
	}
	return out
}

func rowPrimaryKeys(row reflect.Value, primaryColumns []string) (map[string]any, bool) {
	if row.Kind() == reflect.Pointer {
		if row.IsNil() {
			return nil, false
		}
		row = row.Elem()
	}
	if row.Kind() != reflect.Struct {
		return nil, false
	}
	keys := make(map[string]any, len(primaryColumns))
	for _, column := range primaryColumns {
		value, ok := fieldByDBName(row, column)
		if !ok || value.IsZero() {
			return nil, false
		}
		keys[column] = value.Interface()
	}
	return keys, true
}

func fieldByDBName(row reflect.Value, column string) (reflect.Value, bool) {
	rowType := row.Type()
	for i := 0; i < row.NumField(); i++ {
		fieldType := rowType.Field(i)
		field := row.Field(i)
		if fieldType.Anonymous {
			if value, ok := fieldByDBName(field, column); ok {
				return value, true
			}
		}
		if columnNameForStructField(fieldType) == column {
			if field.Kind() == reflect.Pointer && field.IsNil() {
				return reflect.Value{}, false
			}
			return field, field.CanInterface()
		}
	}
	return reflect.Value{}, false
}

func columnNameForStructField(field reflect.StructField) string {
	if field.PkgPath != "" && !field.Anonymous {
		return ""
	}
	tag := field.Tag.Get("gorm")
	settings := schema.ParseTagSetting(tag, ";")
	if column := settings["COLUMN"]; column != "" {
		return column
	}
	return schema.NamingStrategy{}.ColumnName("", field.Name)
}

func targetHasPrimaryKey(target *gorm.DB, table string, keys map[string]any) bool {
	where := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, column := range sortedMapKeys(keys) {
		where = append(where, quoteIdent(column)+" = ?")
		args = append(args, keys[column])
	}
	var count int64
	err := target.Raw("SELECT COUNT(1) FROM "+quoteIdent(table)+" WHERE "+strings.Join(where, " AND "), args...).Scan(&count).Error
	return err == nil && count > 0
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sqliteTableExists(db *gorm.DB, table string) (bool, error) {
	var count int64
	if err := db.Raw(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("inspect sqlite table %s: %w", table, err)
	}
	return count > 0, nil
}

func modelTableName(db *gorm.DB, m any) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", err
	}
	return stmt.Schema.Table, nil
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
