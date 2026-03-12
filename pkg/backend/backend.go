package backend

// Backend abstracts raw byte I/O for secret storage.
type Backend interface {
	Read() ([]byte, error)
	Write(data []byte) error
	Exists() (bool, error)
}
