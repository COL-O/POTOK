package contracts

import "errors"

// ВРЕМЕННЫЙ МОСТИК СОВМЕСТИМОСТИ.
//
// Прежняя версия common.go несла четыре ошибки каталога; новая авторская
// версия common.go их не содержит, а catalog.go и factory.go на них
// ссылаются. Определения сохранены здесь с прежними текстами, чтобы пакет
// contracts оставался собираемым до прибытия авторских версий
// catalog.go / suggestion.go / factory.go (шаг 1 «Пути Б»).
// Если авторские файлы определят эти ошибки у себя — этот файл удалить.
var (
	// ErrUnknownProduct — BOM-связь ссылается на неизвестное изделие.
	ErrUnknownProduct = errors.New("contracts: unknown product in BOM edge")
	// ErrNonAdjacentLayer — BOM-связь не между соседними слоями.
	ErrNonAdjacentLayer = errors.New("contracts: BOM layers are not adjacent")
	// ErrBOMCycle — в BOM обнаружен цикл.
	ErrBOMCycle = errors.New("contracts: BOM contains a cycle")
	// ErrEmptyCode — пустой код завода или SKU изделия.
	ErrEmptyCode = errors.New("contracts: empty code")
)
