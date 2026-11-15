package contracts

// PassportResult — вердикт паспорта качества.
type PassportResult string

const (
	PassportPass   PassportResult = "pass"
	PassportFail   PassportResult = "fail"
	PassportRework PassportResult = "rework"
)

// QualityPassport — паспорт качества (Pasport из набора front).
type QualityPassport struct {
	PassportID string         `json:"passport_id"`
	FactoryID  uint16         `json:"factory_id"`
	ProductID  uint32         `json:"product_id"`
	Grade      string         `json:"grade"`
	Result     PassportResult `json:"result"`
	IssuedAt   int64          `json:"issued_at"`
}
