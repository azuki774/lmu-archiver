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

SQLiteファイルごとにSteamユーザは1人だけ。最初の取り込み成功時に自動登録し、
以降は異なるSteam IDの取り込みを、既存データを変更せずエラーで中断する。
別のSteamユーザを扱う場合は、`--db` で別のSQLiteファイルを指定する。

## オフライン再取り込み

DBへの取り込みに失敗した場合は、保存された取得ディレクトリを指定して再取り込みする。

```sh
lmu-archiver sync --skip-download --input /path/to/capture
```

LMUの起動やRaceOSへの接続は不要。ファイルに含まれる結果とDR/SR観測を取り込む。

## 参照とHTTP API

取り込み後は `races` でレース結果、`ratings` でDR/SRの履歴を参照する。
ほかのアプリケーションから参照する場合は、`serve` で読み取り専用HTTP APIを起動する。
トラック別の集計とフィルター選択肢は、このHTTP APIでのみ提供する。

```sh
lmu-archiver races
lmu-archiver ratings
lmu-archiver serve
```

参照対象はDB内の唯一のユーザから決まる。環境変数・設定ファイル・参照コマンドでの
ユーザ指定は不要。空のDBでは先に `sync` を実行する。
既に複数のSteamユーザが登録されたDBは、データを削除せずエラーとして扱う。
リクエストからのユーザ切り替えはなく、フロントエンドにユーザ選択UIは不要。
型とサンプルを含む簡潔な仕様は [API reference](lmu-archiver-api.yaml) を参照。

設定とコマンドの詳細は [README](../README.md)、DBは [SQLite reference](sqlite.md) を参照。
