package expressions

// bigquery_nodes.go: builders for Kinds the BigQuery dialect needs, generated from
// the pinned sqlglot v30.17.0 classes.

// Kwarg ports exp.Kwarg.
func Kwarg(args Args) Expression { return newNode(KindKwarg, args) }

// HavingMax ports exp.HavingMax.
func HavingMax(args Args) Expression { return newNode(KindHavingMax, args) }

// AIEmbed ports exp.AIEmbed.
func AIEmbed(args Args) Expression { return newNode(KindAIEmbed, args) }

// AIGenerate ports exp.AIGenerate.
func AIGenerate(args Args) Expression { return newNode(KindAIGenerate, args) }

// AISimilarity ports exp.AISimilarity.
func AISimilarity(args Args) Expression { return newNode(KindAISimilarity, args) }

// ApproxTopK ports exp.ApproxTopK.
func ApproxTopK(args Args) Expression { return newNode(KindApproxTopK, args) }

// SafeFunc ports exp.SafeFunc.
func SafeFunc(args Args) Expression { return newNode(KindSafeFunc, args) }

// NetFunc ports exp.NetFunc.
func NetFunc(args Args) Expression { return newNode(KindNetFunc, args) }

// Export ports exp.Export.
func Export(args Args) Expression { return newNode(KindExport, args) }

// ForIn ports exp.ForIn.
func ForIn(args Args) Expression { return newNode(KindForIn, args) }

// Predict ports exp.Predict.
func Predict(args Args) Expression { return newNode(KindPredict, args) }

// GenerateEmbedding ports exp.GenerateEmbedding.
func GenerateEmbedding(args Args) Expression { return newNode(KindGenerateEmbedding, args) }

// MLTranslate ports exp.MLTranslate.
func MLTranslate(args Args) Expression { return newNode(KindMLTranslate, args) }

// MLForecast ports exp.MLForecast.
func MLForecast(args Args) Expression { return newNode(KindMLForecast, args) }

// AIForecast ports exp.AIForecast.
func AIForecast(args Args) Expression { return newNode(KindAIForecast, args) }

// FeaturesAtTime ports exp.FeaturesAtTime.
func FeaturesAtTime(args Args) Expression { return newNode(KindFeaturesAtTime, args) }

// VectorSearch ports exp.VectorSearch.
func VectorSearch(args Args) Expression { return newNode(KindVectorSearch, args) }

// MakeInterval ports exp.MakeInterval.
func MakeInterval(args Args) Expression { return newNode(KindMakeInterval, args) }

// CurrentDatetime ports exp.CurrentDatetime.
func CurrentDatetime(args Args) Expression { return newNode(KindCurrentDatetime, args) }

// JSONBool ports exp.JSONBool.
func JSONBool(args Args) Expression { return newNode(KindJSONBool, args) }

// Contains ports exp.Contains.
func Contains(args Args) Expression { return newNode(KindContains, args) }

// DateFromParts ports exp.DateFromParts.
func DateFromParts(args Args) Expression { return newNode(KindDateFromParts, args) }

// TsOrDsToDatetime ports exp.TsOrDsToDatetime.
func TsOrDsToDatetime(args Args) Expression { return newNode(KindTsOrDsToDatetime, args) }

// TsOrDsToTimestamp ports exp.TsOrDsToTimestamp.
func TsOrDsToTimestamp(args Args) Expression { return newNode(KindTsOrDsToTimestamp, args) }

// TsOrDsToTime ports exp.TsOrDsToTime.
func TsOrDsToTime(args Args) Expression { return newNode(KindTsOrDsToTime, args) }

// Datetime ports exp.Datetime.
func Datetime(args Args) Expression { return newNode(KindDatetime, args) }

// TimestampFromParts ports exp.TimestampFromParts.
func TimestampFromParts(args Args) Expression { return newNode(KindTimestampFromParts, args) }

// TimeFromParts ports exp.TimeFromParts.
func TimeFromParts(args Args) Expression { return newNode(KindTimeFromParts, args) }

// WeekStart ports exp.WeekStart.
func WeekStart(args Args) Expression { return newNode(KindWeekStart, args) }

// JSONStripNulls ports exp.JSONStripNulls.
func JSONStripNulls(args Args) Expression { return newNode(KindJSONStripNulls, args) }

// ParseDatetime ports exp.ParseDatetime.
func ParseDatetime(args Args) Expression { return newNode(KindParseDatetime, args) }

// ParseTime ports exp.ParseTime.
func ParseTime(args Args) Expression { return newNode(KindParseTime, args) }

// SHA1Digest ports exp.SHA1Digest.
func SHA1Digest(args Args) Expression { return newNode(KindSHA1Digest, args) }

// Normalize ports exp.Normalize.
func Normalize(args Args) Expression { return newNode(KindNormalize, args) }

// ByteLength ports exp.ByteLength.
func ByteLength(args Args) Expression { return newNode(KindByteLength, args) }

// LowerHex ports exp.LowerHex.
func LowerHex(args Args) Expression { return newNode(KindLowerHex, args) }

// JSONKeysAtDepth ports exp.JSONKeysAtDepth.
func JSONKeysAtDepth(args Args) Expression { return newNode(KindJSONKeysAtDepth, args) }

// JSONValueArray ports exp.JSONValueArray.
func JSONValueArray(args Args) Expression { return newNode(KindJSONValueArray, args) }

// JSONExtractArray ports exp.JSONExtractArray.
func JSONExtractArray(args Args) Expression { return newNode(KindJSONExtractArray, args) }

// BitwiseAndAgg ports exp.BitwiseAndAgg.
func BitwiseAndAgg(args Args) Expression { return newNode(KindBitwiseAndAgg, args) }

// BitwiseOrAgg ports exp.BitwiseOrAgg.
func BitwiseOrAgg(args Args) Expression { return newNode(KindBitwiseOrAgg, args) }

// BitwiseXorAgg ports exp.BitwiseXorAgg.
func BitwiseXorAgg(args Args) Expression { return newNode(KindBitwiseXorAgg, args) }

// BitwiseCount ports exp.BitwiseCount.
func BitwiseCount(args Args) Expression { return newNode(KindBitwiseCount, args) }

// IntDiv ports exp.IntDiv.
func IntDiv(args Args) Expression { return newNode(KindIntDiv, args) }

// Timestamp ports exp.Timestamp.
func Timestamp(args Args) Expression { return newNode(KindTimestamp, args) }

// DatetimeTrunc ports exp.DatetimeTrunc.
func DatetimeTrunc(args Args) Expression { return newNode(KindDatetimeTrunc, args) }

// DatetimeDiff ports exp.DatetimeDiff.
func DatetimeDiff(args Args) Expression { return newNode(KindDatetimeDiff, args) }

// DatetimeAdd ports exp.DatetimeAdd.
func DatetimeAdd(args Args) Expression { return newNode(KindDatetimeAdd, args) }

// DatetimeSub ports exp.DatetimeSub.
func DatetimeSub(args Args) Expression { return newNode(KindDatetimeSub, args) }

// TimeAdd ports exp.TimeAdd.
func TimeAdd(args Args) Expression { return newNode(KindTimeAdd, args) }

// TimeSub ports exp.TimeSub.
func TimeSub(args Args) Expression { return newNode(KindTimeSub, args) }

// TimestampAdd ports exp.TimestampAdd.
func TimestampAdd(args Args) Expression { return newNode(KindTimestampAdd, args) }

// TimestampSub ports exp.TimestampSub.
func TimestampSub(args Args) Expression { return newNode(KindTimestampSub, args) }

// LastDay ports exp.LastDay.
func LastDay(args Args) Expression { return newNode(KindLastDay, args) }

// SafeDivide ports exp.SafeDivide.
func SafeDivide(args Args) Expression { return newNode(KindSafeDivide, args) }

// Int64 ports exp.Int64.
func Int64(args Args) Expression { return newNode(KindInt64, args) }

// GenerateText ports exp.GenerateText.
func GenerateText(args Args) Expression { return newNode(KindGenerateText, args) }

// GenerateTable ports exp.GenerateTable.
func GenerateTable(args Args) Expression { return newNode(KindGenerateTable, args) }

// GenerateBool ports exp.GenerateBool.
func GenerateBool(args Args) Expression { return newNode(KindGenerateBool, args) }

// GenerateInt ports exp.GenerateInt.
func GenerateInt(args Args) Expression { return newNode(KindGenerateInt, args) }

// GenerateDouble ports exp.GenerateDouble.
func GenerateDouble(args Args) Expression { return newNode(KindGenerateDouble, args) }

// Time ports exp.Time.
func Time(args Args) Expression { return newNode(KindTime, args) }

// DateSub ports exp.DateSub.
func DateSub(args Args) Expression { return newNode(KindDateSub, args) }

// VolatileProperty ports exp.VolatileProperty.
func VolatileProperty(args Args) Expression { return newNode(KindVolatileProperty, args) }

// ArgMax ports exp.ArgMax.
func ArgMax(args Args) Expression { return newNode(KindArgMax, args) }

// ArgMin ports exp.ArgMin.
func ArgMin(args Args) Expression { return newNode(KindArgMin, args) }

// SHA ports exp.SHA.
func SHA(args Args) Expression { return newNode(KindSHA, args) }

// TimestampDiff ports exp.TimestampDiff.
func TimestampDiff(args Args) Expression { return newNode(KindTimestampDiff, args) }

// UnixDate ports exp.UnixDate.
func UnixDate(args Args) Expression { return newNode(KindUnixDate, args) }

// Uuid ports exp.Uuid.
func Uuid(args Args) Expression { return newNode(KindUuid, args) }

// JSONArray ports exp.JSONArray.
func JSONArray(args Args) Expression { return newNode(KindJSONArray, args) }

// Translate ports exp.Translate.
func Translate(args Args) Expression { return newNode(KindTranslate, args) }

// Pseudocolumn ports exp.Pseudocolumn.
func Pseudocolumn(args Args) Expression { return newNode(KindPseudocolumn, args) }

// TimeDiff ports exp.TimeDiff.
func TimeDiff(args Args) Expression { return newNode(KindTimeDiff, args) }

// TimeTrunc ports exp.TimeTrunc.
func TimeTrunc(args Args) Expression { return newNode(KindTimeTrunc, args) }
