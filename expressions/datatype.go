package expressions

import (
	"fmt"
	"strings"
)

type DType string

const (
	DTypeArray                   DType = "ARRAY"
	DTypeAggregateFunction       DType = "AGGREGATEFUNCTION"
	DTypeSimpleAggregateFunction DType = "SIMPLEAGGREGATEFUNCTION"
	DTypeBigDecimal              DType = "BIGDECIMAL"
	DTypeBigInt                  DType = "BIGINT"
	DTypeBigNum                  DType = "BIGNUM"
	DTypeBigSerial               DType = "BIGSERIAL"
	DTypeBinary                  DType = "BINARY"
	DTypeBit                     DType = "BIT"
	DTypeBlob                    DType = "BLOB"
	DTypeBoolean                 DType = "BOOLEAN"
	DTypeBpchar                  DType = "BPCHAR"
	DTypeChar                    DType = "CHAR"
	DTypeCharacterSet            DType = "CHARACTER_SET"
	DTypeDate                    DType = "DATE"
	DTypeDate32                  DType = "DATE32"
	DTypeDateMultiRange          DType = "DATEMULTIRANGE"
	DTypeDateRange               DType = "DATERANGE"
	DTypeDatetime                DType = "DATETIME"
	DTypeDatetime2               DType = "DATETIME2"
	DTypeDatetime64              DType = "DATETIME64"
	DTypeDecimal                 DType = "DECIMAL"
	DTypeDecimal32               DType = "DECIMAL32"
	DTypeDecimal64               DType = "DECIMAL64"
	DTypeDecimal128              DType = "DECIMAL128"
	DTypeDecimal256              DType = "DECIMAL256"
	DTypeDecFloat                DType = "DECFLOAT"
	DTypeDouble                  DType = "DOUBLE"
	DTypeDynamic                 DType = "DYNAMIC"
	DTypeEnum                    DType = "ENUM"
	DTypeEnum8                   DType = "ENUM8"
	DTypeEnum16                  DType = "ENUM16"
	DTypeFile                    DType = "FILE"
	DTypeFixedString             DType = "FIXEDSTRING"
	DTypeFloat                   DType = "FLOAT"
	DTypeGeography               DType = "GEOGRAPHY"
	DTypeGeographyPoint          DType = "GEOGRAPHYPOINT"
	DTypeGeometry                DType = "GEOMETRY"
	DTypePoint                   DType = "POINT"
	DTypeRing                    DType = "RING"
	DTypeLineString              DType = "LINESTRING"
	DTypeMultiLineString         DType = "MULTILINESTRING"
	DTypePolygon                 DType = "POLYGON"
	DTypeMultiPolygon            DType = "MULTIPOLYGON"
	DTypeHllSketch               DType = "HLLSKETCH"
	DTypeHstore                  DType = "HSTORE"
	DTypeImage                   DType = "IMAGE"
	DTypeInet                    DType = "INET"
	DTypeInt                     DType = "INT"
	DTypeInt128                  DType = "INT128"
	DTypeInt256                  DType = "INT256"
	DTypeInt4MultiRange          DType = "INT4MULTIRANGE"
	DTypeInt4Range               DType = "INT4RANGE"
	DTypeInt8MultiRange          DType = "INT8MULTIRANGE"
	DTypeInt8Range               DType = "INT8RANGE"
	DTypeInterval                DType = "INTERVAL"
	DTypeIpAddress               DType = "IPADDRESS"
	DTypeIpPrefix                DType = "IPPREFIX"
	DTypeIpv4                    DType = "IPV4"
	DTypeIpv6                    DType = "IPV6"
	DTypeJSON                    DType = "JSON"
	DTypeJSONB                   DType = "JSONB"
	DTypeList                    DType = "LIST"
	DTypeLongBlob                DType = "LONGBLOB"
	DTypeLongText                DType = "LONGTEXT"
	DTypeLowCardinality          DType = "LOWCARDINALITY"
	DTypeMap                     DType = "MAP"
	DTypeMediumBlob              DType = "MEDIUMBLOB"
	DTypeMediumInt               DType = "MEDIUMINT"
	DTypeMediumText              DType = "MEDIUMTEXT"
	DTypeMoney                   DType = "MONEY"
	DTypeName                    DType = "NAME"
	DTypeNChar                   DType = "NCHAR"
	DTypeNested                  DType = "NESTED"
	DTypeNothing                 DType = "NOTHING"
	DTypeNull                    DType = "NULL"
	DTypeNumMultiRange           DType = "NUMMULTIRANGE"
	DTypeNumRange                DType = "NUMRANGE"
	DTypeNVarchar                DType = "NVARCHAR"
	DTypeObject                  DType = "OBJECT"
	DTypeRange                   DType = "RANGE"
	DTypeRowVersion              DType = "ROWVERSION"
	DTypeSerial                  DType = "SERIAL"
	DTypeSet                     DType = "SET"
	DTypeSmallDatetime           DType = "SMALLDATETIME"
	DTypeSmallInt                DType = "SMALLINT"
	DTypeSmallMoney              DType = "SMALLMONEY"
	DTypeSmallSerial             DType = "SMALLSERIAL"
	DTypeStruct                  DType = "STRUCT"
	DTypeSuper                   DType = "SUPER"
	DTypeText                    DType = "TEXT"
	DTypeTinyBlob                DType = "TINYBLOB"
	DTypeTinyText                DType = "TINYTEXT"
	DTypeTime                    DType = "TIME"
	DTypeTimeTz                  DType = "TIMETZ"
	DTypeTimeNs                  DType = "TIME_NS"
	DTypeTimestamp               DType = "TIMESTAMP"
	DTypeTimestampNtz            DType = "TIMESTAMPNTZ"
	DTypeTimestampLtz            DType = "TIMESTAMPLTZ"
	DTypeTimestampTz             DType = "TIMESTAMPTZ"
	DTypeTimestampS              DType = "TIMESTAMP_S"
	DTypeTimestampMs             DType = "TIMESTAMP_MS"
	DTypeTimestampNs             DType = "TIMESTAMP_NS"
	DTypeTinyInt                 DType = "TINYINT"
	DTypeTsMultiRange            DType = "TSMULTIRANGE"
	DTypeTsRange                 DType = "TSRANGE"
	DTypeTstzMultiRange          DType = "TSTZMULTIRANGE"
	DTypeTstzRange               DType = "TSTZRANGE"
	DTypeUBigInt                 DType = "UBIGINT"
	DTypeUInt                    DType = "UINT"
	DTypeUInt128                 DType = "UINT128"
	DTypeUInt256                 DType = "UINT256"
	DTypeUMediumInt              DType = "UMEDIUMINT"
	DTypeUDecimal                DType = "UDECIMAL"
	DTypeUDouble                 DType = "UDOUBLE"
	DTypeUnion                   DType = "UNION"
	DTypeUnknown                 DType = "UNKNOWN"
	DTypeUserDefined             DType = "USER-DEFINED"
	DTypeUSmallInt               DType = "USMALLINT"
	DTypeUTinyInt                DType = "UTINYINT"
	DTypeUUID                    DType = "UUID"
	DTypeVarBinary               DType = "VARBINARY"
	DTypeVarchar                 DType = "VARCHAR"
	DTypeVariant                 DType = "VARIANT"
	DTypeVector                  DType = "VECTOR"
	DTypeXML                     DType = "XML"
	DTypeYear                    DType = "YEAR"
	DTypeTDigest                 DType = "TDIGEST"
)

var dTypeByName = map[string]DType{
	"ARRAY":                   DTypeArray,
	"AGGREGATEFUNCTION":       DTypeAggregateFunction,
	"SIMPLEAGGREGATEFUNCTION": DTypeSimpleAggregateFunction,
	"BIGDECIMAL":              DTypeBigDecimal,
	"BIGINT":                  DTypeBigInt,
	"BIGNUM":                  DTypeBigNum,
	"BIGSERIAL":               DTypeBigSerial,
	"BINARY":                  DTypeBinary,
	"BIT":                     DTypeBit,
	"BLOB":                    DTypeBlob,
	"BOOLEAN":                 DTypeBoolean,
	"BPCHAR":                  DTypeBpchar,
	"CHAR":                    DTypeChar,
	"CHARACTER_SET":           DTypeCharacterSet,
	"DATE":                    DTypeDate,
	"DATE32":                  DTypeDate32,
	"DATEMULTIRANGE":          DTypeDateMultiRange,
	"DATERANGE":               DTypeDateRange,
	"DATETIME":                DTypeDatetime,
	"DATETIME2":               DTypeDatetime2,
	"DATETIME64":              DTypeDatetime64,
	"DECIMAL":                 DTypeDecimal,
	"DECIMAL32":               DTypeDecimal32,
	"DECIMAL64":               DTypeDecimal64,
	"DECIMAL128":              DTypeDecimal128,
	"DECIMAL256":              DTypeDecimal256,
	"DECFLOAT":                DTypeDecFloat,
	"DOUBLE":                  DTypeDouble,
	"DYNAMIC":                 DTypeDynamic,
	"ENUM":                    DTypeEnum,
	"ENUM8":                   DTypeEnum8,
	"ENUM16":                  DTypeEnum16,
	"FILE":                    DTypeFile,
	"FIXEDSTRING":             DTypeFixedString,
	"FLOAT":                   DTypeFloat,
	"GEOGRAPHY":               DTypeGeography,
	"GEOGRAPHYPOINT":          DTypeGeographyPoint,
	"GEOMETRY":                DTypeGeometry,
	"POINT":                   DTypePoint,
	"RING":                    DTypeRing,
	"LINESTRING":              DTypeLineString,
	"MULTILINESTRING":         DTypeMultiLineString,
	"POLYGON":                 DTypePolygon,
	"MULTIPOLYGON":            DTypeMultiPolygon,
	"HLLSKETCH":               DTypeHllSketch,
	"HSTORE":                  DTypeHstore,
	"IMAGE":                   DTypeImage,
	"INET":                    DTypeInet,
	"INT":                     DTypeInt,
	"INT128":                  DTypeInt128,
	"INT256":                  DTypeInt256,
	"INT4MULTIRANGE":          DTypeInt4MultiRange,
	"INT4RANGE":               DTypeInt4Range,
	"INT8MULTIRANGE":          DTypeInt8MultiRange,
	"INT8RANGE":               DTypeInt8Range,
	"INTERVAL":                DTypeInterval,
	"IPADDRESS":               DTypeIpAddress,
	"IPPREFIX":                DTypeIpPrefix,
	"IPV4":                    DTypeIpv4,
	"IPV6":                    DTypeIpv6,
	"JSON":                    DTypeJSON,
	"JSONB":                   DTypeJSONB,
	"LIST":                    DTypeList,
	"LONGBLOB":                DTypeLongBlob,
	"LONGTEXT":                DTypeLongText,
	"LOWCARDINALITY":          DTypeLowCardinality,
	"MAP":                     DTypeMap,
	"MEDIUMBLOB":              DTypeMediumBlob,
	"MEDIUMINT":               DTypeMediumInt,
	"MEDIUMTEXT":              DTypeMediumText,
	"MONEY":                   DTypeMoney,
	"NAME":                    DTypeName,
	"NCHAR":                   DTypeNChar,
	"NESTED":                  DTypeNested,
	"NOTHING":                 DTypeNothing,
	"NULL":                    DTypeNull,
	"NUMMULTIRANGE":           DTypeNumMultiRange,
	"NUMRANGE":                DTypeNumRange,
	"NVARCHAR":                DTypeNVarchar,
	"OBJECT":                  DTypeObject,
	"RANGE":                   DTypeRange,
	"ROWVERSION":              DTypeRowVersion,
	"SERIAL":                  DTypeSerial,
	"SET":                     DTypeSet,
	"SMALLDATETIME":           DTypeSmallDatetime,
	"SMALLINT":                DTypeSmallInt,
	"SMALLMONEY":              DTypeSmallMoney,
	"SMALLSERIAL":             DTypeSmallSerial,
	"STRUCT":                  DTypeStruct,
	"SUPER":                   DTypeSuper,
	"TEXT":                    DTypeText,
	"TINYBLOB":                DTypeTinyBlob,
	"TINYTEXT":                DTypeTinyText,
	"TIME":                    DTypeTime,
	"TIMETZ":                  DTypeTimeTz,
	"TIME_NS":                 DTypeTimeNs,
	"TIMESTAMP":               DTypeTimestamp,
	"TIMESTAMPNTZ":            DTypeTimestampNtz,
	"TIMESTAMPLTZ":            DTypeTimestampLtz,
	"TIMESTAMPTZ":             DTypeTimestampTz,
	"TIMESTAMP_S":             DTypeTimestampS,
	"TIMESTAMP_MS":            DTypeTimestampMs,
	"TIMESTAMP_NS":            DTypeTimestampNs,
	"TINYINT":                 DTypeTinyInt,
	"TSMULTIRANGE":            DTypeTsMultiRange,
	"TSRANGE":                 DTypeTsRange,
	"TSTZMULTIRANGE":          DTypeTstzMultiRange,
	"TSTZRANGE":               DTypeTstzRange,
	"UBIGINT":                 DTypeUBigInt,
	"UINT":                    DTypeUInt,
	"UINT128":                 DTypeUInt128,
	"UINT256":                 DTypeUInt256,
	"UMEDIUMINT":              DTypeUMediumInt,
	"UDECIMAL":                DTypeUDecimal,
	"UDOUBLE":                 DTypeUDouble,
	"UNION":                   DTypeUnion,
	"UNKNOWN":                 DTypeUnknown,
	"USERDEFINED":             DTypeUserDefined,
	"USMALLINT":               DTypeUSmallInt,
	"UTINYINT":                DTypeUTinyInt,
	"UUID":                    DTypeUUID,
	"VARBINARY":               DTypeVarBinary,
	"VARCHAR":                 DTypeVarchar,
	"VARIANT":                 DTypeVariant,
	"VECTOR":                  DTypeVector,
	"XML":                     DTypeXML,
	"YEAR":                    DTypeYear,
	"TDIGEST":                 DTypeTDigest,
}

func DTypeFromName(name string) (DType, bool) {
	d, ok := dTypeByName[name]
	return d, ok
}

func IsType(e Expression, t DType) bool {
	return e != nil && e.Kind() == KindDataType && e.Arg("this") == t
}

func DataType(args Args) Expression      { return newNode(KindDataType, args) }
func DataTypeParam(args Args) Expression { return newNode(KindDataTypeParam, args) }
func Interval(args Args) Expression      { return newNode(KindInterval, args) }
func IntervalSpan(args Args) Expression  { return newNode(KindIntervalSpan, args) }

var StructTypes = map[DType]bool{
	DTypeFile:   true,
	DTypeNested: true,
	DTypeObject: true,
	DTypeStruct: true,
	DTypeUnion:  true,
}

var ArrayTypes = map[DType]bool{
	DTypeArray: true,
	DTypeList:  true,
}

var NestedTypes = map[DType]bool{
	DTypeFile:   true,
	DTypeNested: true,
	DTypeObject: true,
	DTypeStruct: true,
	DTypeUnion:  true,
	DTypeArray:  true,
	DTypeList:   true,
	DTypeMap:    true,
}

var TextTypes = map[DType]bool{
	DTypeChar:     true,
	DTypeNChar:    true,
	DTypeNVarchar: true,
	DTypeText:     true,
	DTypeVarchar:  true,
	DTypeName:     true,
}

var SignedIntegerTypes = map[DType]bool{
	DTypeBigInt:    true,
	DTypeInt:       true,
	DTypeInt128:    true,
	DTypeInt256:    true,
	DTypeMediumInt: true,
	DTypeSmallInt:  true,
	DTypeTinyInt:   true,
}

var UnsignedIntegerTypes = map[DType]bool{
	DTypeUBigInt:    true,
	DTypeUInt:       true,
	DTypeUInt128:    true,
	DTypeUInt256:    true,
	DTypeUMediumInt: true,
	DTypeUSmallInt:  true,
	DTypeUTinyInt:   true,
}

var IntegerTypes = map[DType]bool{
	DTypeBigInt:     true,
	DTypeInt:        true,
	DTypeInt128:     true,
	DTypeInt256:     true,
	DTypeMediumInt:  true,
	DTypeSmallInt:   true,
	DTypeTinyInt:    true,
	DTypeUBigInt:    true,
	DTypeUInt:       true,
	DTypeUInt128:    true,
	DTypeUInt256:    true,
	DTypeUMediumInt: true,
	DTypeUSmallInt:  true,
	DTypeUTinyInt:   true,
	DTypeBit:        true,
}

var FloatTypes = map[DType]bool{
	DTypeDouble: true,
	DTypeFloat:  true,
}

var RealTypes = map[DType]bool{
	DTypeDouble:     true,
	DTypeFloat:      true,
	DTypeBigDecimal: true,
	DTypeDecimal:    true,
	DTypeDecimal32:  true,
	DTypeDecimal64:  true,
	DTypeDecimal128: true,
	DTypeDecimal256: true,
	DTypeDecFloat:   true,
	DTypeMoney:      true,
	DTypeSmallMoney: true,
	DTypeUDecimal:   true,
	DTypeUDouble:    true,
}

var NumericTypes = map[DType]bool{
	DTypeBigInt:     true,
	DTypeInt:        true,
	DTypeInt128:     true,
	DTypeInt256:     true,
	DTypeMediumInt:  true,
	DTypeSmallInt:   true,
	DTypeTinyInt:    true,
	DTypeUBigInt:    true,
	DTypeUInt:       true,
	DTypeUInt128:    true,
	DTypeUInt256:    true,
	DTypeUMediumInt: true,
	DTypeUSmallInt:  true,
	DTypeUTinyInt:   true,
	DTypeBit:        true,
	DTypeDouble:     true,
	DTypeFloat:      true,
	DTypeBigDecimal: true,
	DTypeDecimal:    true,
	DTypeDecimal32:  true,
	DTypeDecimal64:  true,
	DTypeDecimal128: true,
	DTypeDecimal256: true,
	DTypeDecFloat:   true,
	DTypeMoney:      true,
	DTypeSmallMoney: true,
	DTypeUDecimal:   true,
	DTypeUDouble:    true,
}

var TemporalTypes = map[DType]bool{
	DTypeDate:          true,
	DTypeDate32:        true,
	DTypeDatetime:      true,
	DTypeDatetime2:     true,
	DTypeDatetime64:    true,
	DTypeSmallDatetime: true,
	DTypeTime:          true,
	DTypeTimestamp:     true,
	DTypeTimestampNtz:  true,
	DTypeTimestampLtz:  true,
	DTypeTimestampTz:   true,
	DTypeTimestampMs:   true,
	DTypeTimestampNs:   true,
	DTypeTimestampS:    true,
	DTypeTimeTz:        true,
}

func (d DType) IntoExpr(kwargs Args) Expression {
	return applyKwargs(DataType(Args{"this": d}), kwargs)
}

func maybeCopy(e Expression, copyValue bool) Expression {
	if copyValue && e != nil {
		return e.Copy()
	}
	return e
}

func DataTypeBuild(dtype any, dialect string, udt bool, copyValue bool, kwargs Args) (Expression, error) {
	switch v := dtype.(type) {
	case string:
		return DataTypeFromStr(v, dialect, udt, kwargs)
	case DType:
		return applyKwargs(DataType(Args{"this": v}), kwargs), nil
	case Expression:
		if v.Kind() == KindDataType {
			return maybeCopy(v, copyValue), nil
		}
		if udt && (v.Kind() == KindIdentifier || v.Kind() == KindDot) {
			return applyKwargs(DataType(Args{"this": DTypeUserDefined, "kind": v}), kwargs), nil
		}
	}
	return nil, fmt.Errorf("Invalid data type: %T. Expected str or DType", dtype)
}

func DataTypeFromStr(dtype, dialect string, udt bool, kwargs Args) (Expression, error) {
	if strings.ToUpper(dtype) == "UNKNOWN" {
		return applyKwargs(DataType(Args{"this": DTypeUnknown}), kwargs), nil
	}
	if ParseIntoFunc == nil {
		return nil, fmt.Errorf("expressions.ParseIntoFunc is not configured")
	}
	expr, err := ParseIntoFunc(dtype, dialect, KindDataType, true)
	if err != nil {
		if udt {
			return applyKwargs(DataType(Args{"this": DTypeUserDefined, "kind": dtype}), kwargs), nil
		}
		return nil, err
	}
	return applyKwargs(expr, kwargs), nil
}

func DataTypeIsType(e Expression, checkNullable bool, dtypes ...any) bool {
	if e == nil {
		return false
	}
	self := e
	if self.Kind() != KindDataType {
		self = asExpression(e.Arg("to"))
		if self == nil || self.Kind() != KindDataType {
			return false
		}
	}
	selfNullable := truthy(self.Arg("nullable"))
	for _, dtype := range dtypes {
		other, err := DataTypeBuild(dtype, "", true, false, nil)
		if err != nil || other == nil {
			continue
		}
		otherNullable := truthy(other.Arg("nullable"))
		var matches bool
		if len(other.Expressions()) > 0 || (checkNullable && (selfNullable || otherNullable)) || self.Arg("this") == DTypeUserDefined || other.Arg("this") == DTypeUserDefined {
			matches = self.Equal(other)
		} else {
			matches = self.Arg("this") == other.Arg("this")
		}
		if matches {
			return true
		}
	}
	return false
}
