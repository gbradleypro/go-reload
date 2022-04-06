package gin

type Config struct {
	Laddr    string `json:"laddr"`
	Port     int    `json:"port"`
	ProxyTo  string `json:"proxy_to"`
	KeyFile  string `json:"key_file"`
	CertFile string `json:"cert_file"`
}
