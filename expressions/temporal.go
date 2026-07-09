package expressions

// StrToDate/StrToTime/StrToUnix/TimeStrToDate/TimeStrToTime/TimeStrToUnix port the
// STR_TO_*/TIME_STR_TO_* temporal family (expressions/temporal.py:452-473), each a plain
// one-line constructor mirroring e.g. json.go's JSONExtract/JSONCast builders above.
func StrToDate(args Args) Expression     { return newNode(KindStrToDate, args) }
func StrToTime(args Args) Expression     { return newNode(KindStrToTime, args) }
func StrToUnix(args Args) Expression     { return newNode(KindStrToUnix, args) }
func TimeStrToDate(args Args) Expression { return newNode(KindTimeStrToDate, args) }
func TimeStrToTime(args Args) Expression { return newNode(KindTimeStrToTime, args) }
func TimeStrToUnix(args Args) Expression { return newNode(KindTimeStrToUnix, args) }
