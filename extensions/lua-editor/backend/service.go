// 编辑器业务通过宿主动作和声明式表运行，平台入口不接触源码与存储规则。
package luaeditor

import ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"

func Service() ext.Service {
	drafts := &draftService{gate: make(chan struct{}, 1)}
	return ext.Service{Actions: map[string]ext.ServiceHandler{
		"analyze":      sourceAnalysis,
		"draft-read":   drafts.serial(drafts.read),
		"draft-save":   drafts.serial(drafts.save),
		"draft-delete": drafts.serial(drafts.remove),
	}}
}
