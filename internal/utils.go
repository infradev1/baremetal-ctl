package internal

func SplitIntoChunks(data []byte, chunkSize int) [][]byte {
	var chunks [][]byte

	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		chunks = append(chunks, data[i:end])
	}

	return chunks
}
