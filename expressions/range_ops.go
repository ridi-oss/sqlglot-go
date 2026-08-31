package expressions

// range-ops cluster builders: parser/parser.go's parseRange/parseIs/columnOperators build
// these Kinds directly via the constructors below (mirroring e.g. Like/ILike/SimilarTo in
// core.go). See the KindGlob..KindSoundex const-block comment in kinds.go for the exact
// upstream class each one ports.
func Glob(args Args) Expression                    { return newNode(KindGlob, args) }
func Overlaps(args Args) Expression                { return newNode(KindOverlaps, args) }
func RegexpILike(args Args) Expression             { return newNode(KindRegexpILike, args) }
func Adjacent(args Args) Expression                { return newNode(KindAdjacent, args) }
func ExtendsLeft(args Args) Expression             { return newNode(KindExtendsLeft, args) }
func ExtendsRight(args Args) Expression            { return newNode(KindExtendsRight, args) }
func ArrayContainsAll(args Args) Expression        { return newNode(KindArrayContainsAll, args) }
func ArrayContainedBy(args Args) Expression        { return newNode(KindArrayContainedBy, args) }
func ArrayOverlaps(args Args) Expression           { return newNode(KindArrayOverlaps, args) }
func JSONBContains(args Args) Expression           { return newNode(KindJSONBContains, args) }
func JSONBContainsAllTopKeys(args Args) Expression { return newNode(KindJSONBContainsAllTopKeys, args) }
func JSONBContainsAnyTopKeys(args Args) Expression { return newNode(KindJSONBContainsAnyTopKeys, args) }
func JSONBDeleteAtPath(args Args) Expression       { return newNode(KindJSONBDeleteAtPath, args) }
func JSONBPathExists(args Args) Expression         { return newNode(KindJSONBPathExists, args) }
func JSON(args Args) Expression                    { return newNode(KindJSON, args) }
func Operator(args Args) Expression                { return newNode(KindOperator, args) }
func MatchAgainst(args Args) Expression            { return newNode(KindMatchAgainst, args) }
func JSONArrayContains(args Args) Expression       { return newNode(KindJSONArrayContains, args) }
func Soundex(args Args) Expression                 { return newNode(KindSoundex, args) }
