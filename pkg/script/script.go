package script

// type SplitFunc func(data []byte, atEOF bool) (advance int, token []byte, err error)
//     SplitFunc is the signature of the split function used to tokenize the
//     input. The arguments are an initial substring of the remaining
//     unprocessed data and a flag, atEOF, that reports whether the Reader has
//     no more data to give. The return values are the number of bytes to
//     advance the input and the next token to return to the user, plus an
//     error, if any. If the data does not yet hold a complete token, for
//     instance if it has no newline while scanning lines, SplitFunc can return
//     (0, nil) to signal the Scanner to read more data into the slice and try
//     again with a longer slice starting at the same point in the input.
// 
//     If the returned error is non-nil, scanning stops and the error is
//     returned to the client.
// 
//     The function is never called with an empty data slice unless atEOF is
//     true. If atEOF is true, however, data may be non-empty and, as always,
//     holds unprocessed text.
// 


func ScanScript(data []byte, atEOF bool) (int, []byte, error) {
	data = bytes.TrimLeft(data, " \t")
	if len(data) == 0 {
		return 0, nil, nil
	}
}


{
	var buf []byte
	for chunk := range input {
		// We only start a new iteration at the token buondary
		buf = buf + chunk
		// Skip whitespaces
		buf = bytes.TrimLeft(data, " \t")
		if len(buf) == 0 {
			continue
		}
		wordRE := regexp.Compile("^[^ \t{}\\]*")
		wordRE.Longest()
		 wordMatch := wordRE.FindIndex(buf)
		if wordMatch != nil {
			word := word
		}
		if buf[0] 

	}

}
