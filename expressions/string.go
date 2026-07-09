package expressions

// Chr ports exp.Chr (string.py:23-26): `class Chr(Expression, Func)`, arg_types=
// {"expressions": True, "charset": False}, is_var_len_args=True, _sql_names=["CHR", "CHAR"].
// Built directly by parser.parseChar (special USING-clause grammar), not via FromArgList.
func Chr(args Args) Expression { return newNode(KindChr, args) }
