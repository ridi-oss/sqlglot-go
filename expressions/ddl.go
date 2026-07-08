package expressions

func Create(args Args) Expression  { return newNode(KindCreate, args) }
func Command(args Args) Expression { return newNode(KindCommand, args) }

// Builders for the 19 statement-family Kinds (see the block comment in kinds.go for the
// upstream class/line references); each is a one-line constructor per the node-model
// convention (AGENTS.md:34-43).
func Set(args Args) Expression            { return newNode(KindSet, args) }
func SetItem(args Args) Expression        { return newNode(KindSetItem, args) }
func Show(args Args) Expression           { return newNode(KindShow, args) }
func Use(args Args) Expression            { return newNode(KindUse, args) }
func Kill(args Args) Expression           { return newNode(KindKill, args) }
func Describe(args Args) Expression       { return newNode(KindDescribe, args) }
func LoadData(args Args) Expression       { return newNode(KindLoadData, args) }
func Transaction(args Args) Expression    { return newNode(KindTransaction, args) }
func Commit(args Args) Expression         { return newNode(KindCommit, args) }
func Rollback(args Args) Expression       { return newNode(KindRollback, args) }
func Grant(args Args) Expression          { return newNode(KindGrant, args) }
func Revoke(args Args) Expression         { return newNode(KindRevoke, args) }
func GrantPrivilege(args Args) Expression { return newNode(KindGrantPrivilege, args) }
func GrantPrincipal(args Args) Expression { return newNode(KindGrantPrincipal, args) }
func Comment(args Args) Expression        { return newNode(KindComment, args) }
func TruncateTable(args Args) Expression  { return newNode(KindTruncateTable, args) }
func Partition(args Args) Expression      { return newNode(KindPartition, args) }
func Analyze(args Args) Expression        { return newNode(KindAnalyze, args) }
func Pragma(args Args) Expression         { return newNode(KindPragma, args) }

// FileFormatProperty backs the `FORMAT=<fmt>` option on DESCRIBE (properties.py:176).
func FileFormatProperty(args Args) Expression { return newNode(KindFileFormatProperty, args) }
