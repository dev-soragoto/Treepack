# treepack

`treepack` 是一个基于 `kit.toml` 的文件树编排打包工具。

它会把 GitHub release、直接 URL、本地文件或本地目录组合到一个输出目录里，生成构建报告，并可选按稳定 entry 顺序打包成 zip。

```text
读取配置 -> 解析资源 -> 安装资源 -> 执行文件操作 -> 验证 -> 写报告 -> 按配置可选生成临时 zip -> 发布 output -> 按配置可选替换正式 zip
```

## 快速开始

运行内置 smoke 示例：

```powershell
treepack -c examples/smoke/kit.toml
```

示例命令显式读取：

```text
examples/smoke/kit.toml
```

并生成：

```text
examples/smoke-out/
examples/smoke-2026.07.07.zip
```

在自己的 kit 仓库里使用时，通常在包含 `kit.toml` 的目录运行：

```powershell
treepack
```

## 开发

本仓库使用 `mise` 管理 Go 工具链、`goimports` 和 GoReleaser。

```powershell
mise install
mise run fmt
go run ./cmd/treepack -c examples/smoke/kit.toml
goreleaser check
goreleaser release --snapshot --clean
```

## 命令行

```powershell
treepack [-c kit.toml] [-s DIR] [-o DIR] [--work-dir DIR] [--keep-work] [--disable-cache] [--raw-archive] [--explain] [-p PROXY] [--download-retries N] [--github-token TOKEN] [-v] [-h]
```

参数：

- `-c`, `--config`：manifest 路径，默认 `kit.toml`。
- `-s`, `--source`：覆盖 `paths.source`。
- `-o`, `--output`：覆盖 `paths.output`。
- `--work-dir`：覆盖 `paths.work`。
- `--keep-work`：保留本次构建 work run dir。
- `--disable-cache`：禁止读取和写入持久下载缓存。
- `--raw-archive`：生成 zip 时按本次 staged output 的内容原样归档，保留默认会过滤的系统元数据。
- `--explain`：按真实执行顺序输出静态操作计划，不读取 source/ZIP、不访问网络，也不创建 work/output。
- `-p`, `--proxy`：下载代理，例如 `http://127.0.0.1:7890` 或 `socks5://127.0.0.1:7890`。
- `--download-retries`：下载总尝试次数，包含首次请求，默认 `3`。网络请求错误以及 `429`、`502`、`503`、`504` 状态码会重试，并遵循 `Retry-After`。
- `-v`, `--version`：显示版本号。
- `-h`, `--help`：显示总览帮助。主题帮助使用 flag 形式，例如 `treepack --help config`、`treepack -h packages`。可用主题包括 `config`、`packages`、`build`、`paths`、`verify`、`examples`。
- `--github-token`：用于 GitHub release API 和 GitHub asset 下载的 token。推荐优先使用环境变量。

GitHub token 优先级：

1. `--github-token`
2. `GITHUB_TOKEN`
3. `GH_TOKEN`

代理规则：

- 未传 `-p` / `--proxy` 时，下载使用 Go 默认代理环境变量：`HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY` 及其小写形式。
- 传入 `-p` / `--proxy` 时，只使用显式代理，不读取代理环境变量。
- 显式代理支持 `http://`、`https://`、`socks5://`、`socks5h://`。
- 不支持 `ALL_PROXY` / `all_proxy`。

输出约定：

- `stdout`：只输出命令主要结果，例如构建成功行。
- `stderr`：进度、诊断、warning、error。

`paths.output`（以及 CLI 的 `-o` / `--output`）是构建结果目录，不是受保护的存储目录。一旦 Treepack 进入输出发布阶段，会先删除 output 中原有的全部内容，再写入本次构建结果。

Treepack 不保留上一次成功构建的 output，也不提供事务、回滚或失败恢复。配置了 archive 时会先从 staged output 创建临时 zip，zip 创建成功后才发布 final output；但如果 final output 发布、archive rename 或进程中断失败，命令可能以失败状态退出，output 仍可能已被删除，或包含部分本次构建产生的文件和目录。不要将重要文件、手工维护的内容或唯一副本放在 output 中。

archive 临时文件使用最终 archive 目录内的随机名称，不占用或删除固定的 `<archive>.tmp`。ZIP 创建失败时不会发布新 output，也不会替换旧 archive，只会清理本次随机临时文件。

Treepack 主要保证文件内容、目录结构、覆盖语义、路径边界和 ZIP 结构安全。它不限制下载大小、ZIP 解压大小、ZIP entry 数量、压缩比、磁盘占用或耗时；磁盘空间、资源可信度和运行环境容量由使用者自行控制。

复制与解压会按当前操作系统能力尽量保留普通文件权限，但权限属于 best-effort 行为。Treepack 不保证 ACL、owner/group、时间戳、扩展属性或跨平台权限一致，也不复制 Windows ACL。

Windows 上会拒绝 symlink、junction、mount point 等会改变文件树边界的链接型入口。OneDrive、Dropbox、cloud placeholder 和同步目录不作为安全边界问题；可读时按普通文件或目录处理，不可读时按普通 I/O 失败处理。

只有在 `kit.toml` 的 `[build]` 中声明 `archive` 时才会创建 zip。省略 `archive` 会只生成输出目录和报告，适合 GitHub Actions 等 CI 交给上传步骤自动打包的场景。配置 archive 时，Treepack 会先从本次 staged output 创建临时 zip；创建成功后才发布 final output，并在发布后用临时 zip 替换正式 archive。生成 zip 时默认跳过常见 macOS、Windows 和 Linux 桌面元数据；此过滤只影响 zip 内容，不改变发布到 `paths.output` 的内容。需要按本次 staged output 的内容原样归档时使用 `--raw-archive`。

退出码：

- `0`：成功。
- `1`：manifest、source、build、operation 或 verify 失败。
- `2`：参数错误或未知命令。

## 路径解析规则

`--config` 的相对路径按当前工作目录解析。manifest 读取成功后，manifest 里的 `[paths].source`、`[paths].output`、`[paths].work` 和 `[build].archive` 的相对路径都按 `kit.toml` 所在目录解析。

例如当前目录是仓库根目录时：

```powershell
treepack -c examples/smoke/kit.toml
```

那么 `examples/smoke/kit.toml` 里的：

```toml
[paths]
source = "."
output = "../smoke-out"
work = "../.treepack/work"

[build]
archive = "../smoke-{pack.version}.zip"
```

会解析为：

```text
<仓库根目录>/examples/smoke
<仓库根目录>/examples/smoke-out
<仓库根目录>/examples/.treepack/work
<仓库根目录>/examples/smoke-2026.07.07.zip
```

如果你从其他目录运行同一个 `--config <path-to-kit>`，这些 manifest 内相对路径仍然按 `kit.toml` 所在目录解析，不随当前工作目录变化。

CLI 覆盖路径 `-s` / `--source`、`-o` / `--output`、`--work-dir` 的相对路径按当前工作目录解析，用来明确覆盖 manifest 中的路径。

`file:` source 是第二层路径：它不是相对当前工作目录，而是相对已经解析好的 `paths.source`。例如：

```toml
[paths]
source = "resources"

[[packages]]
source = "file:fixtures/payload.bin"
```

实际读取的是 `<kit.toml 所在目录>/resources/fixtures/payload.bin`，除非你用 CLI 覆盖了 `paths.source`。

输出树内部路径是第三层路径：package `target`、package `steps` 中的 `output/...`、`layout.dirs`、`verify`、`build.build_info` 都相对对应的 staging / final output，不相对当前工作目录，也不相对 `paths.source`。

## kit.toml 示例

```toml
[pack]
name = "Example Pack"
version = "2026.07.07"

[paths]
source = "resources"
output = "out"
work = ".treepack/work"

[build]
archive = "out-{pack.version}.zip"
build_info = "BUILD_INFO.txt"
keep_work = false

[layout]
dirs = [
  "bin",
]

[[packages]]
name = "Local Zip"
required = true
source = "file:fixtures/archive.zip"
asset = 'archive\.zip'
install = "extract"

[[packages]]
name = "Single File"
required = true
source = "file:fixtures/payload.bin"
asset = 'payload\.bin'
target = "bin/payload.bin"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[[packages.steps]]
op = "touch"
path = "output/READY.txt"
required = true

[verify]
files = [
  "READY.txt",
  "bin/payload.bin",
]
dirs = [
  "bin",
]
absent = [
  "old.txt",
]
```

`build.keep_work` 与 CLI `--keep-work` 使用 OR 关系：任一为 `true` 都会保留本次 work run dir。

配置 `paths.work` 或 `--work-dir` 后，Treepack 在 `<work>/cache/` 保存持久下载缓存，并在 `<work>/run-*/` 创建每次构建的临时目录。构建清理只删除本次 `run-*`；`--keep-work` 不影响缓存。没有稳定 work root 时，持久缓存禁用。GitHub 每次仍查询 release 和 asset metadata，仅复用经名称、大小、digest/校验和验证的资产正文；普通 URL 只有配置 SHA-256 时才缓存，命中后不访问网络。`--disable-cache` 同时禁止缓存读写。

## Source 语法

```text
github:owner/repo
github:owner/repo@tag
url:https://example.com/file.zip
file:resources/local.bin
file:resources/local_dir
```

说明：

- `github:owner/repo`：读取 latest release。
- `github:owner/repo@tag`：读取指定 release tag。
- `url:`：直接从指定 HTTP 或 HTTPS 链接下载文件。
- `file:`：读取本地文件或目录。

GitHub release asset 当前仅保证公开 release asset 下载。`--github-token` / `GITHUB_TOKEN` 会用于 release API 请求；私有仓库 release asset 的专用下载流程暂不作为兼容承诺。

`file:` 路径从 `paths.source` 解析，并且必须留在 `paths.source` 内；绝对 `file:` 路径也必须位于该目录内。`file:` 直接指向普通文件时可以不写 `asset`；写了 `asset` 时，正则必须以 Go `MatchString` 语义匹配该文件的 basename。`file:` 指向目录且不写 `asset` / `assets` 时，整个目录会作为一个目录资产安装。`file:` 指向目录且写了 `asset` / `assets` 时，该目录会作为资产池，正则只匹配直接子项名称，匹配结果可以是文件或目录，但必须唯一。

直接 URL 示例：

```toml
[[packages]]
name = "Direct URL"
source = "url:https://example.com/releases/payload.bin"
asset = 'payload\.bin'
target = "bin/payload.bin"
required = true
```

下载进度会输出到 `stderr`，不会写入 `BUILD_INFO.txt`。

一个 `url:` 只能对应一个 source asset：可以使用 package 级 `asset`，也可以使用仅含一项的 `[[packages.assets]]`。两项及以上会在 manifest 校验阶段失败，不会发出 HTTP 请求；多个独立 URL 应拆成多个 package。

## Package 安装

每个 package 会先安装到独立 staging 目录。staging 内有两个约定目录：

- `extract/`：`install = "extract"` 解压出来的原始内容。
- `output/`：这个 package 明确声明要安装到最终输出树的内容。

package 成功后，只有 `output/` 的内容会按 manifest 顺序串行合并到 staged output。后面的 package 可以覆盖前面的文件。`extract/` 里的文件不会自动进入最终输出。

package `steps` 读取和修改当前 package staging，不会 fallback 到 `paths.source`。要引入静态资源，请使用普通 `file:` package；目录资源可用 `target = "."` 合并到输出根：

```toml
[[packages]]
name = "Static Resources"
group = "resources"
required = true
source = "file:resources/sd"
target = "."
```

`asset` 是正则表达式，用来选择 source asset：GitHub release asset、URL 文件名、直接 `file:` 文件的 basename，或 `file:` 本地目录下的直接子项名称。它不用于选择 ZIP 内部 entry。匹配使用 Go `MatchString` 语义，不会自动添加 `^` 或 `$`；匹配结果必须唯一。TOML 里推荐用单引号写正则，避免 `\\.zip` 这种双层转义。

默认安装方式是复制文件本身。单文件 asset 会复制到 package `output/`，`target` 是相对 `output/` 的路径。只有显式写 `install = "extract"` 时，treepack 才会把匹配到的 zip 解压到 package `extract/<safe-asset-name>/`；`extract` 与 `target` 互斥。ZIP 内多个 entry 仍由一个 `install = "extract"` asset 配合多个 package steps 安装。如果同一个 package 内多个 zip asset 的安全目录名发生碰撞，构建会失败，不会静默合并 staging 内容。

本地目录 source 不写 `asset` / `assets` 时会安装整个目录，并默认保留目录名：

```toml
[[packages]]
name = "App"
source = "file:app"
```

上例会安装到最终输出的 `app/`。指定 `target` 会把目录安装到该路径：

```toml
[[packages]]
name = "App"
source = "file:app"
target = "program"
```

`target = "."` 会把目录内容合并到当前 package 的 `output/` 根，而不是创建外层目录：

对单文件资产，`target = "."` 表示放到 `output/` 根并保留文件 basename。

```toml
[[packages]]
name = "App"
source = "file:app"
target = "."
```

目录资产不能使用 `install = "extract"`；`extract` 只支持 zip 文件。

复制并安装单文件：

```toml
[[packages]]
name = "Payload"
source = "github:owner/repo"
asset = 'payload\.bin'
target = "bin/payload.bin"
required = true
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
```

`sha256` 是可选 checksum pin，值必须是 64 位十六进制字符串。它校验下载或复制后的原始 asset 文件；不会校验安装后的目标文件，也不支持目录资产。

只解压 zip，不安装任何文件到输出：

```toml
[[packages]]
name = "Archive"
source = "github:owner/repo"
asset = 'archive.*\.zip'
install = "extract"
required = true
```

解压后如需安装其中的文件，需要在同一个 package 里写 `steps`，把文件从 `extract/...` 显式复制到 `output/...`：

```toml
[[packages]]
name = "Archive"
source = "github:owner/repo"
asset = 'archive.*\.zip'
install = "extract"
required = true

[[packages.steps]]
op = "cp"
from = "extract/archive.zip/path/a.bin"
to = "output/a/a.bin"
required = true
```

从同一个 source 安装多个 release assets：

```toml
[[packages]]
name = "Multi Asset"
source = "github:owner/repo"
required = false

[[packages.assets]]
asset = 'tool-a\.bin'
target = "tools/a.bin"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[[packages.assets]]
asset = 'tool-b\.bin'
target = "tools/b.bin"
sha256 = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
```

如果多个 asset 里既有 zip 又有单文件，可以只给 zip 那一项写 `install = "extract"`：

```toml
[[packages]]
name = "Mixed Assets"
source = "github:owner/repo"

[[packages.assets]]
asset = 'bundle\.zip'
install = "extract"

[[packages.assets]]
asset = 'helper\.bin'
target = "tools/helper.bin"

[[packages.steps]]
op = "cp"
from = "extract/bundle.zip/path/a.bin"
to = "output/a/a.bin"
required = true
```

## 文件操作

支持的操作：

- `cp`
- `cp_regex`
- `rm`
- `touch`

所有操作只能写在 package `steps` 内，并且路径都相对当前 package staging。`extract/` 是解压输入区，`output/` 是该 package 会发布到最终 staged output 的内容。

package 未声明 `required` 时默认为 `true`。step 未声明 `required` 时继承所属 package 的有效值；step 显式值会覆盖继承。有效值为 `false` 的 step 失败时只记录 warning，继续执行后续 steps，并在 package 完成后合并已经产生的 output。有效值为 `true` 的 step 失败时终止当前 package；只有该 package 本身为 required 时才阻断整个构建。

`cp` 使用 literal path，支持文件和目录。复制文件到已存在目录时保留 basename；复制目录到已存在目录时会创建/合并 `to/<basename>`；`from = "dir/."` 表示复制目录内容到 `to`：

```toml
[[packages.steps]]
op = "cp"
from = "extract/archive.zip/path/a.bin"
to = "output/a/a.bin"
required = true

[[packages.steps]]
op = "cp"
from = "extract/archive.zip/overlay/."
to = "output"
required = true
```

`cp_regex` 只匹配 `from` 目录的直接子项名称，不递归搜索；匹配到的文件或目录会复制到 `to/<basename>`：

```toml
[[packages.steps]]
op = "cp_regex"
from = "extract/archive.zip/folder"
regex = '^app_payload.*\.bin$'
to = "output/bin"
required = true
```

ZIP entry 为了跨 Windows、Linux 和 macOS 可落盘，可能在解压前修正名称。`< > : " / \ | ? *`、NUL 和控制字符会替换为 `_`，尾随点/空格逐字符替换为 `_`，Windows 设备名（如 `CON.txt`）会前置 `_`。普通 Unicode、空格和三平台支持的标点保持不变。

`cp.from`、`cp.to`、`rm.path`、`touch.path` 中位于 `extract/<asset>/...` 下的 literal 路径会采用相同修正规则，因此 manifest 仍可引用 ZIP 中的原始名称。例如 `extract/archive.zip/CON.txt` 会解析到实际的 `extract/archive.zip/_CON.txt`。`cp_regex` 则始终匹配解压后的实际物理名称。

`rm` 删除 literal path，文件和目录都支持，路径不存在也视为成功。`touch` 创建 marker 文件并自动创建父目录，文件已存在时不会截断：

```toml
[[packages.steps]]
op = "rm"
path = "output/tools/old-helper.bin"
required = false

[[packages.steps]]
op = "touch"
path = "output/READY.txt"
required = false
```

## 路径安全

`treepack` 默认拒绝会逃出工作目录的路径：

- package `target` 必须留在 output 内。
- `layout.dirs`、`build.build_info` 必须留在 output 内。
- package `steps` 的读写、删除目标必须留在当前 package staging 内。
- `file:` source 必须留在 `paths.source` 内。
- `paths.source`、`paths.output`、`paths.work` 不能相同，也不能互相包含。
- `build.archive` 不能和 `paths.source`、`paths.output`、`paths.work`、本次 work run dir 或 manifest 文件重叠；archive 目标不能是既有目录。
- zip 会先扫描完整 central directory、修正便携名称并检查完整落盘计划，预检通过后才开始解压；预检前不会创建或修改 extract 目标。
- `..`、绝对路径、盘符和 UNC entry，以及 symlink、特殊文件、重复 entry、修正后重名、文件/目录同名、文件父路径、Windows 大小写等价和 Unicode NFC 等价冲突都会拒绝整个 ZIP。
- ZIP 名称修正发生冲突时不会继续编号或增加 hash，而是拒绝整个 ZIP。

上述 package target/steps、layout、verify、build_info、archive 模板及路径重叠会在任何网络请求之前统一预检；错误会带配置位置，例如 `packages[1].assets[1].target`。

配置里的目录路径可以是绝对路径。manifest 相对路径按 `kit.toml` 所在目录解析，CLI 覆盖路径按当前运行目录解析。文件树内部路径不要使用绝对路径或 `..` 穿越路径。

## 输出文件

发布 output 时会删除旧 output 并写入本次结果。output 是可被 Treepack 破坏性重建的产物目录。

构建完成后，output 目录里会生成：

```text
BUILD_INFO.txt
```

`BUILD_INFO.txt` 记录 pack 信息、builder 版本、resolved asset、脱敏后的 asset URL、SHA-256、operations、archive path corrections、verification 和 failures。ZIP entry 自动改名会在 stderr 输出 warning 并写入 `Archive Path Corrections`，但不会计入 failures。发布报告不包含 `Resolved Paths` 段或本机绝对路径；内存报告和 stderr 日志仍保留解析路径用于诊断。

构建失败时，报告不会发布到 output。需要查看失败过程中的 staged report 时，使用 `--keep-work` 保留本次 work run dir。

`verify.files` 要求目标是普通文件；`verify.dirs` 要求目标是目录；`verify.absent` 要求文件或目录都不存在。

ZIP 会写入普通文件和目录 entry，包括空目录。ZIP 内路径统一使用 `/`，entry 顺序稳定；Treepack 不承诺不同操作系统或不同文件元数据下生成字节完全一致的 ZIP。

默认生成的 ZIP 会忽略常见系统元数据：`.DS_Store`、`._*`、`__MACOSX/`、`.AppleDouble/`、`Thumbs.db`、`ehthumbs.db`、`Desktop.ini`、`.directory`、`$RECYCLE.BIN/`、`System Volume Information/` 和 `.Trash-*/`。匹配按 entry 的文件名或目录名判断，不按路径子串判断；过滤目录会跳过整棵子树。此过滤只影响 `[build].archive` 生成的 zip，不影响最终 `paths.output` 目录。使用 `--raw-archive` 可关闭该过滤。

archive 文件名支持：

```text
{pack.name}
{pack.safe_name}
{pack.version}
```

这些变量都会先做路径安全清洗：只保留字母、数字、`.`、`_`、`-`，其他字符替换为 `_`；空值、`.`、`..` 变为 `_`，尾随点变为 `_`，Windows 设备名会前置 `_`。未知占位符会报错；不支持把原始 pack 字符串直接拼进路径。

## 静态执行说明

`treepack --explain` 读取并校验 manifest，解析 config/source/output/work/archive 和 CLI 路径覆盖，然后逐项展示 asset 安装或解压、package steps、package merge、layout、verify、build info、archive 与 publish。箭头左侧是该操作的直接目标，右侧是直接来源；它不会推演最终合并树。

Explain 不打开 ZIP，因此 ZIP 节点显示 `<archive contents not inspected>`，无法列出归档实际会发生的 correction；但 manifest literal 路径中可静态确定的便携修正会单独显示。`cp_regex` 仍按解压后的物理名称解释。
