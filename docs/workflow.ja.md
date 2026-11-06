# ワークフロー

[English](workflow.md)

## 取得と保存

LMUを起動した状態で `lmu-archiver sync` を実行する。

1. 起動中のLMUからSteamセッションチケットを取得し、RaceOSで認証する。
2. 認証プロフィールから現在のDR/SRとSteam IDを取り出す。
3. レース結果の全履歴をRaceOSから取得する。
4. 取得した結果とプロフィールを、再取り込み用のファイルとして保存する。
5. 自分のレース結果をSQLiteへ取り込み、DR/SRは変化があった場合だけ保存する。

```text
LMU → RaceOS認証 → プロフィール・レース結果の取得 → ファイル保存 → SQLiteへの取り込み
```

## オフライン再取り込み

DBへの取り込みに失敗した場合は、保存された取得ディレクトリを指定して再取り込みする。

```sh
lmu-archiver sync --skip-download --input /path/to/capture
```

LMUの起動やRaceOSへの接続は不要。ファイルに含まれる結果とDR/SR観測を取り込む。

## 参照とHTTP API

取り込み後は `races` でレース結果、`ratings` でDR/SRの履歴を参照する。
ほかのアプリケーションから参照する場合は、`serve` で読み取り専用HTTP APIを起動する。

```sh
lmu-archiver races
lmu-archiver ratings
lmu-archiver serve
```

HTTP APIの対象はサーバ起動時に1人へ固定する。`serve --steam-id`、
環境変数 `STEAM_ID`、設定ファイルの `steamId`、DB内の唯一のユーザの順で決まり、
起動後は変更しない。空の `STEAM_ID` は設定の `steamId` を消し、その値には戻らない。
`.env` は自動では読まないため、シェルでexportしてから起動する。
解決後のIDが空でDB内のユーザが0人または複数なら、待ち受け開始前にエラーになる。
リクエストからのユーザ切り替えはなく、フロントエンドにユーザ選択UIは不要。
型とサンプルを含む簡潔な仕様は [API reference](lmu-archiver-api.yaml) を参照。

設定とコマンドの詳細は [README](../README.md)、DBは [SQLite reference](sqlite.md) を参照。
