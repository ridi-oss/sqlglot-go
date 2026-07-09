package expressions

func JSONExtract(args Args) Expression        { return newNode(KindJSONExtract, args) }
func JSONExtractScalar(args Args) Expression  { return newNode(KindJSONExtractScalar, args) }
func JSONBExtract(args Args) Expression       { return newNode(KindJSONBExtract, args) }
func JSONBExtractScalar(args Args) Expression { return newNode(KindJSONBExtractScalar, args) }
func JSONCast(args Args) Expression           { return newNode(KindJSONCast, args) }
func JSONTable(args Args) Expression          { return newNode(KindJSONTable, args) }
func JSONColumnDef(args Args) Expression      { return newNode(KindJSONColumnDef, args) }
func JSONSchema(args Args) Expression         { return newNode(KindJSONSchema, args) }
func FormatJson(args Args) Expression         { return newNode(KindFormatJson, args) }
func JSONObject(args Args) Expression         { return newNode(KindJSONObject, args) }
func JSONObjectAgg(args Args) Expression      { return newNode(KindJSONObjectAgg, args) }
func JSONKeyValue(args Args) Expression       { return newNode(KindJSONKeyValue, args) }
func OnCondition(args Args) Expression        { return newNode(KindOnCondition, args) }
func JSONValue(args Args) Expression          { return newNode(KindJSONValue, args) }
