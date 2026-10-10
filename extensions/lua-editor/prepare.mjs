// 构建输出与签名输入分离，签名私钥由调用者显式提供。
import {copyFile,mkdir,writeFile} from 'node:fs/promises'
import {execFileSync} from 'node:child_process'
await mkdir('package',{recursive:true})
const python = process.env.CPH_PYTHON || (process.platform === 'win32' ? 'python' : 'python3')
await writeFile('package/manifest.json', execFileSync(python, ['../../scripts/project_config.py', 'manifest', 'lua-editor']))
await copyFile('../../LICENSE','package/LICENSE')
await writeFile('package/icon.svg','<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 48 48"><rect x="1" y="1" width="46" height="46" rx="8" fill="#1a2440"/><text x="24" y="30" text-anchor="middle" fill="#edf0f6" font-family="monospace" font-size="18">Lua</text></svg>')
