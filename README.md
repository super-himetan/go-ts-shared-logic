# go-ts-shared-logic

バックエンドの Go のコードを、フロントエンドの TypeScript からそのまま import する実験です。

```ts
import { Price, type Order } from "../shared/checkout/checkout.go";

const quote = Price(order); // 型は Go の宣言から、実行はブラウザの Wasm で
```

金額計算と入力チェックの実装は [shared/checkout/checkout.go](shared/checkout/checkout.go) の1か所だけです。コード生成したファイルをコミットすることはなく、Go を変えると TypeScript 側がすぐ型エラーになります。

TypeScript 7.1 ベータの [Content Mapper](https://devblogs.microsoft.com/typescript/announcing-typescript-7-1-beta/) を使っています。

## 試す

Go 1.27 以降、Node.js、pnpm が必要です。[mise](https://mise.jdx.dev/) を使っていれば `mise install` ですべて入ります。

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

本番と同じ形で動かすときは、ビルドしてからサーバーだけを起動し、http://localhost:8787 を開きます。

```bash
pnpm build
```

```bash
pnpm server
```

`pnpm build` は Wasm の Brotli 版と gzip 版も作り、サーバーはブラウザが対応する形式で返します。`assets/` のファイルは名前にハッシュが入っているので、長期間キャッシュされます。

## Go を変えてみる

`Quote.Total` のタグを `json:"grandTotal"` に変えて、型チェックを実行します。

```bash
pnpm check
```

```text
web/app.ts(51,30): error TS2339: Property 'total' does not exist on type 'Quote'.
```

JSON に載せられない型を使うと、Go のファイルの該当箇所がエラーになります。

```text
shared.go(8,21): error go9201: This type has no JSON form that TypeScript can describe.
```

## 仕組み

```text
checkout.go ─┬ tsc / エディタ → Content Mapper → goschema → 型
             ├ Vite プラグイン → goschema と GOOS=js GOARCH=wasm go build → Wasm
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
| `tests/` | Wasm と型のテスト |
| `e2e/` | 本番ビルドをブラウザで動かす Playwright のテスト |

Content Mapper が作るのは型だけで、実行時には使われません。実行時のモジュールは Vite プラグインが作ります。

`shared/checkout/testdata/cases.json` は Go のテストと Wasm のテストの両方で読み、同じ結果になることを確かめています。

## 型の対応

`encoding/json` が実際に書き出す形に合わせています。

| Go | TypeScript |
| --- | --- |
| `string`、`bool` | `string`、`boolean` |
| `int`、`int32`、`float64` など | `number` |
| `[]T` | `T[] \| null` |
| `map[string]T`、`map[int]T` | `{ [__K in string]?: T } \| null`、`{ [__K in number]?: T } \| null` |
| `struct { A int }` | `{ A: number }` |
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
- 実行時は同じパッケージのファイルが1つの Wasm と Go ランタイムを共有します。関数のないファイルだけを import した場合、Wasm は読み込みません。
- `int64`、`uint64`、`uint` は JavaScript の数値で正確に表せない場合があるので、エラーにしています。`int` も Wasm では64ビットなので、2^53 を超える値は正確に渡せません。
- `int` の引数に `1.5` や `1e30` を渡すと例外になります。TypeScript の型はどちらも `number` なので、コンパイル時には検出できません。
- `MarshalJSON` などで JSON の形を変える型はエラーにしていますが、別のファイルに書かれたメソッドは検出できません。
- `const` の union は同じファイルの定数だけから作ります。`Color("blue")` のような変換、別のファイルの定数、ゼロ値の `""` は型に現れません。
- ジェネリクス、可変長引数、埋め込みフィールド、リテラル以外の公開 `const` には対応していません。
- `packages/go-mapper` はこのリポジトリの `bridge` と `tools/goschema` に依存していて、単体では配布できません。
- `tsc` は mapper のエラーがあると、その先の型エラーを報告しません。
- 共有できるのは純粋なロジックだけです。DB やファイルを使うコードは Wasm では動きません。
- `pnpm dev` が自動で再ビルドするのは、import したファイルと同じパッケージの変更だけです。別のパッケージを変えたときは、dev サーバーを再起動します。
- 通常の Go で作る Wasm は大きく、このデモで約 4.5MB（Brotli で約 0.92MB、gzip で約 1.25MB）あります。読み込むまで import 元のモジュールは実行されないので、画面は `web/main.ts` から動的 import し、読み込み中と失敗の表示を出しています。

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

ブラウザのテストは、初回だけブラウザのインストールが必要です。ポート 8787 と 5186 を空けて実行してください。

```bash
pnpm exec playwright install chromium
```

```bash
pnpm e2e
```
