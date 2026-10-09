package extension

// SystemComponent 由平台查询，不能通过文件型扩展卸载接口删除。
type SystemComponent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Package   string `json:"package"`
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
	Available bool   `json:"available"`
	Execution string `json:"execution"`
}
