package expressions

func Copy(args Args) Expression          { return newNode(KindCopy, args) }
func CopyParameter(args Args) Expression { return newNode(KindCopyParameter, args) }
func Credentials(args Args) Expression   { return newNode(KindCredentials, args) }
func Unload(args Args) Expression        { return newNode(KindUnload, args) }
