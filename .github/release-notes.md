## 安装（macOS）

1. 按芯片下载下面的 `.dmg`（不确定就点屏幕左上角的苹果菜单 →「关于本机」看「芯片」或「处理器」）：
   - **Apple 芯片**（M1、M2、M3、M4…）：`mymind-x.y.z-arm64.dmg`
   - **Intel 处理器**：`mymind-x.y.z-amd64.dmg`（不是每个版本都有；这一版没有的话，用最近一个带它的版本）

   打开后把 **MyMind** 拖进「应用程序」。
2. 应用没有经过苹果公证，第一次打开前在「终端」执行：

   ```sh
   xattr -cr /Applications/MyMind.app
   ```

3. 双击打开。

## 安装（Windows）

下载下面的 `.exe` 安装包安装（不是每个版本都有；这一版没有的话，用最近一个带它的版本）。应用没有代码签名，Windows 可能提示「已保护你的电脑」，点「更多信息 → 仍要运行」。快捷键里的 ⌘ 在 Windows 上对应 Ctrl。

## 一键更新

装好 `gh`（`brew install gh && gh auth login`）后，用仓库里的 `update.sh` 更新，不会弹安全提示：

```sh
~/mindmap-update.sh
```

第一次获取脚本的方法见仓库 README 的「一键更新」。

## 语义检索插件

「收集」里「按意思搜索」用的离线模型不在安装包里，第一次打开「收集」时点「下载」（约 23MB，或在 设置 → 资料整理 里下载）。模型从公开仓库 [mindmap-models](https://github.com/wuyunyoujiduo-jpg/mindmap-models) 下载，GitHub 不通时自动改用 jsDelivr 镜像，下载后逐块校验。
