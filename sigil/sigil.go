package sigil

// func IndexTxn(rawtx []byte, blockId string, height uint32, idx uint64, dryRun bool) (ctx *lib.IndexContext) {
// 	ctx, err := lib.ParseTxn(rawtx, blockId, height, idx)
// 	if err != nil {
// 		log.Panicln(err)
// 	}
// 	ParseSigil(ctx)
// 	for _, txo := range ctx.Txos {
// 		txo.Save()
// 	}
// 	return
// }

// func ParseSigil(ctx *lib.IndexContext) {
// 	for _, txo := range ctx.Txos {
// 		if txo.Owner != nil && len(*txo.Owner) != 0 {
// 			continue
// 		}
// 		sigil := ParseScript(txo)
// 		if sigil != nil {
// 			txo.AddData("sigil", sigil)
// 		}
// 	}
// }

// func ParseScript(txo *lib.Txo) (sigil *json.RawMessage) {
// 	script := *txo.Tx.Outputs[txo.Outpoint.Vout()].LockingScript

// 	if len(script) > 49 &&
// 		script[0] == bscript.OpHASH160 &&
// 		script[22] == bscript.OpEQUALVERIFY &&
// 		script[23] == bscript.OpDUP &&
// 		script[24] == bscript.OpHASH160 &&
// 		script[46] == bscript.OpEQUALVERIFY &&
// 		script[47] == bscript.OpCHECKSIG &&
// 		script[48] == bscript.OpRETURN {

// 		pkhash := lib.PKHash(script[26:46])
// 		txo.Owner = &pkhash
// 		pos := 49
// 		op, err := lib.ReadOp(script, &pos)
// 		if err == nil {
// 			var s json.RawMessage
// 			err = json.Unmarshal(op.Data, &s)
// 			if err == nil {
// 				sigil = &s
// 			}
// 		}
// 		// OP_DUP OP_HASH160 ea5b697ebacbf7c679bae7ab743c8bdffe885581 OP_EQUALVERIFY OP_2 OP_SWAP 022cde271100c7164068931b5d62b38e13d92d1d7e1d354426b86c328a71952b29 OP_2 OP_CHECKMULTISIG OP_RETURN 7b227469746c65223a2254656e646965222c22696d616765223a22623a2f2f33366361663732653830346136356532353737663834323162353264343839376336376661346362333534613838333733363530353161373034306630333931222c22676c624d6f64656c223a22623a2f2f37663265316534376264353963393338613062613566316132383437383336313933373065633162343837396434336433393566376236356430356135363833227d
// 	} else if len(script) > 64 &&
// 		script[0] == bscript.OpDUP &&
// 		script[1] == bscript.OpHASH160 &&
// 		script[23] == bscript.OpEQUALVERIFY &&
// 		script[24] == bscript.Op2 &&
// 		script[25] == bscript.OpSWAP &&
// 		script[60] == bscript.Op2 &&
// 		script[61] == bscript.OpCHECKMULTISIG &&
// 		script[62] == bscript.OpRETURN {
// 		pkhash := lib.PKHash(script[3:23])
// 		txo.Owner = &pkhash
// 		pos := 63
// 		op, err := lib.ReadOp(script, &pos)
// 		if err == nil {
// 			var s json.RawMessage
// 			err = json.Unmarshal(op.Data, &s)
// 			if err == nil {
// 				sigil = &s
// 			}
// 		}
// 	}
// 	return
// }
