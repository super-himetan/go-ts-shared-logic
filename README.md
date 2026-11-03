# go-ts-shared-logic

バックエンドの Go のコードを、フロントエンドの TypeScript からそのまま import する実験です。

```ts
import { Price, type Order } from "../shared/checkout/checkout.go";

const quote = Price(order); // 型は Go の宣言から、実行はブラウザの Wasm で
```

金額計算と入力チェックの実装は [shared/checkout/checkout.go](shared/checkout/checkout.go) の1か所だけです。コード生成したファイルをコミットすることはなく、Go を変えると TypeScript 側がすぐ型エラーになります。

TypeScript 7.1 ベータの [Content Mapper](https://devblogs.microsoft.com/typescript/announcing-typescript-7-1-beta/) を使っています。

## 試す

Go 1.27 以降、Node.js、pnpm が必要です。[mise](https://mise.jdx.dev/) を使っていれば `mise install` で Go と Node.js が入ります。

```bash
pnpm install
```

```bash
pnpm server
```

別のターミナルで次を実行し、http://localhost:5184 を開きます。

```bash
pnpm dev
```

数量やクーポンを変えると、リクエストなしで金額とエラーが更新されます。「表示より安い金額で送る」を押すと、サーバーが同じコードで計算し直して 409 で拒否します。

## Go を変えてみる

`Quote.Total` のタグを `json:"grandTotal"` に変えて、型チェックを実行します。

```bash
pnpm check
```

```text
web/main.ts(50,30): error TS2339: Property 'total' does not exist on type 'Quote'.
```

JSON に載せられない型を使うと、Go のファイルの該当箇所がエラーになります。

```text
shared.go(8,21): error go9201: This type cannot be sent through JSON to TypeScript.
```

## 仕組み

```text
                         ┌ tools/goschema (go/parser) ┐
checkout.go ─┬ tsc / エディタ → Content Mapper → 型
             ├ Vite → プラグイン → GOOS=js GOARCH=wasm go build → Wasm
             └ server/main.go → 普通の import
```

| 場所 | 役割 |
| --- | --- |
| `shared/checkout/` | 共有するロジック。ただの Go のパッケージ |
| `server/` | バックエンド。`checkout` を普通に呼ぶ |
| `web/` | フロントエンド。`checkout.go` を import する |
| `tools/goschema/` | Go ファイルの公開宣言を、TypeScript の型と元の位置に変換する |
| `packages/go-mapper/` | Content Mapper 本体、Vite プラグイン、Wasm のローダー |
| `bridge/` | JavaScript からの呼び出しを JSON で受ける Go 側の入口 |

Content Mapper が作るのは型だけで、実行時には使われません。実行時のモジュールは Vite プラグインが作ります。

`shared/checkout/testdata/cases.json` は Go のテストと Wasm のテストの両方で読み、同じ結果になることを確かめています。

## 型の対応

`encoding/json` が実際に書き出す形に合わせています。

| Go | TypeScript |
| --- | --- |
| `string`、`bool` | `string`、`boolean` |
| `int`、`int32`、`float64` など | `number` |
| `[]T` | `T[] \| null` |
| `map[string]T` | `Partial<Record<string, T>> \| null` |
| `*T` | `T \| null` |
| `time.Time` | `string` |
| `any` | `unknown` |
| `type Size string` と `const SizeS Size = "S"` | `"S" \| ...` |
| `` `json:"name"` `` | プロパティ名 `name` |
| `` `json:",omitempty"` `` | 省略可能なプロパティ |
| `func F(a A) (B, error)` | `function F(a: A): B`。`error` は例外になる |

`nil` のスライスは JSON で `null` になるので、型にも `null` を残しています。

## 制限

- TypeScript `7.1.0-beta` が前提です。
- 型として見えるのは import したファイルの宣言だけです。公開する型は同じファイルに置きます。別のファイルやパッケージの型を公開関数で使うとエラーになります。
- `int64`、`uint64`、`uint` は JavaScript の数値で正確に表せない場合があるので、エラーにしています。`int` も Wasm では64ビットなので、2^53 を超える値は正確に渡せません。
- ジェネリクス、可変長引数、埋め込みフィールド、リテラル以外の `const` には対応していません。
- `tsc` は mapper のエラーがあると、その先の型エラーを報告しません。
- 共有できるのは純粋なロジックだけです。DB やファイルを使うコードは Wasm では動きません。
- `pnpm dev` が自動で再ビルドするのは、import したファイルと同じパッケージの変更だけです。別のパッケージを変えたときは、dev サーバーを再起動します。
- 通常の Go で作る Wasm は大きく、このデモで約 4.5MB（gzip で約 1.3MB）あります。読み込むまで import 元のモジュールは実行されません。

## エディタ

VS Code で使うには、拡張機能 `TypeScriptTeam.native-preview` を入れ、このフォルダを開きます。mapper と Go 拡張は `go` を実行するので、VS Code から `go` にパスが通っている必要があります。

Dock から起動した VS Code は、ホームディレクトリのシェルの PATH を使います。このリポジトリの `mise.toml` だけでは `go` が見えず、Go 拡張が `Failed to find the "go" binary` と表示します。次を実行してから VS Code を完全に終了し、起動し直してください。

```bash
mise use -g go@1.27
```

## 開発

```bash
pnpm check
```

```bash
pnpm test
```
