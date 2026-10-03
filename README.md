# Gitcord

DiscordライクなチャットUIで操作するGitクライアントです。

Gitディレクトリを左のDM欄に並べ、コミット履歴をチャットのように読み、メッセージ送信欄からコミットできます。

## 特徴

- **チャット風の履歴表示**: 下が新しく、上に行くほど古いコミット。スクロールで最初のコミットまで遡れます
- **未プッシュの強調**: まだpushしていないコミットは色を変えて表示
- **履歴の編集**: コミットを右クリックして、未プッシュなら `reset`(soft / mixed / hard)、プッシュ済みなら `revert`
- **コミット時点の閲覧**: コミットをクリックすると、その時点のファイル構造が見られ、ファイルをクリックするとdiffを表示
- **ステージ操作**: 未コミットの変更を一覧し、チェックボックスでステージを切り替え
- **プレフィックス付きコミット**: `feat` `fix` `docs` `style` `refactor` `perf` `test` `chore` のボタンをワンクリックで付与。プレフィックスなしの場合も `none` を押す必要があり、押し忘れによるコミットを防ぎます
- **コミットメッセージの自動英訳**: ON/OFFを切り替え可能(デフォルトON)
- **Gitディレクトリの追加**: アプリ内のファイルマネージャーで選択(Gitディレクトリにはバッジ表示)、またはcloneして追加(clone先とディレクトリ名を指定)
- **認証は既存の設定をそのまま利用**: 内部で `git` コマンドを呼ぶだけなので、credential helperやSSH鍵の設定がそのまま使えます

## インストール

対応OSは現在Linuxのみです。[Releases](../../releases) から、ディストリビューションに合ったファイルをダウンロードしてください。

| 形式 | 主な対象 |
| --- | --- |
| `.AppImage` | ほとんどのディストリビューション |
| `.deb` | Debian / Ubuntu系 |
| `.rpm` | Fedora / openSUSE系 |

AppImageは実行権限を付けて起動します。

```
chmod +x Gitcord_*.AppImage
./Gitcord_*.AppImage
```

実行には `git` がインストールされている必要があります。

## 使い方

1. 左上の「＋」からGitディレクトリを追加します。既存のディレクトリを選ぶか、URLを指定してcloneします。
2. 左の一覧からディレクトリを選ぶと、中央にコミット履歴が表示されます。
3. 右の「変更」タブで、コミットしたいファイルにチェックを入れてステージします。ファイル名をクリックするとdiffが開きます。
4. 下の入力欄にメッセージを書き、プレフィックス(または `none`)を選んで「コミット」を押します。
5. 右上の「push」ボタンでpushします。

メッセージは Ctrl+Enter でも送信できます。

## 注意事項

- **英訳機能とプライバシー**: Googleの非公式の翻訳エンドポイントにコミットメッセージの文面を送信します。機密性の高い内容を扱う場合は、英訳をOFFにしてください。OFFにすると、ネットワークには何も送りません。
- **データの保存場所**: 追加したディレクトリの一覧は `~/.config/gitcord/repos.json` に保存されます。
- **ローカル通信のみ**: 内部のサーバーは `127.0.0.1` でのみ待ち受け、外部からは接続できません。

## トラブルシューティング

### Waylandで起動しない(`Error 71 (Protocol error) dispatching to Wayland display`)

WebKitGTKとの相性問題で、NVIDIA環境などで起きることがあります。次の環境変数を付けて起動してみてください。

```
GDK_BACKEND=x11 ./Gitcord_*.AppImage
```

AppImageで解決しない場合は、お使いのディストリビューションで [ソースからビルド](#ソースからビルド) すると、システムのWebKitGTKが使われるため安定することがあります。

## 既知の制限

- Linuxのみ対応です(Windows・macOSは未対応)
- 履歴は直近5000件までを一括で読み込みます
- 履歴編集コマンドは reset と revert のみです(rebase、amendなどは未対応)
- cloneの進捗は表示されません

## ソースからビルド

必要なもの: Go 1.21以上、Rust(stable)、Node.js、[Tauriの依存ライブラリ](https://v2.tauri.app/start/prerequisites/#linux)

```
TRIPLE=$(rustc -vV | sed -n 's/host: //p')
mkdir -p src-tauri/binaries
CGO_ENABLED=0 go build -ldflags="-s -w" -o src-tauri/binaries/gitcord-server-$TRIPLE .
npm install
npx tauri build --no-bundle
./src-tauri/target/release/gitcord
```

Tauriなしでバックエンドだけを動かして、ブラウザで試すこともできます。

```
go build -o gitcord . && ./gitcord   # http://127.0.0.1:8484
```

### 構成

```
main.go          Goバックエンド(gitコマンドの実行とHTTP API)
index.html       UI本体(1ファイル。Goバイナリに埋め込み)
src-tauri/       Tauriのラッパー(Goバックエンドをsidecarとして起動し、ウィンドウで開く)
web/             Tauriの設定上必要な起動中スプラッシュ
```

UIを変更するなら `index.html`、Git操作やAPIを変更するなら `main.go` を編集します。
