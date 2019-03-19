# Open Container Initiative Runtime Specification 入門

ここまでで、コンテナの概要と、実際に Linux 上でプロセスを用いたコンテナがどのように実装されているかを簡単に確認しました。

実際には、最初に説明したように、コンテナランタイムの仕様は標準化されており、
それに則って実装を行うことで、様々なプラットフォームで動かすことが可能になっています。

ここでは、最初に説明した [opencontainers/runtime-spec](https://github.com/opencontainers/runtime-spec) 並びに、
[opencontainers/image-spec](https://github.com/opencontainers/image-spec) の詳細について確認していきます。

また、実際に手を動かして `runtime-spec` に最低限準拠したコンテナランタイムを作成し、
Linux 上の Docker のランタイムとして使うことで、実際の動作を確認していきます。

## image-spec

[opencontainers/image-spec](https://github.com/opencontainers/image-spec/blob/v1.0.1/spec.md) はコンテナイメージに関する標準化の文章です。
実際の仕様自体は [`spec.md`](https://github.com/opencontainers/image-spec/blob/v1.0.1/spec.md) に書かれています。

コンテナイメージは、例えば開発環境で動作確認が済んだものを本番環境にデプロイするように、様々な場所に転送されます。
そのため、圧縮を行ったり、`layer` と呼ばれるレイヤ化によって、イメージの更新があった箇所だけダウンロードを行えるようにしたりと、高速に転送を行える工夫などがなされています。

今回は image-spec に関して詳細には紹介しませんが、興味がある方はぜひ読んでみてください。

### Docker image から image-spec 準拠のコンテナイメージへの変換

image-spec で標準化が進んでいるものの、現在広く使われている Docker のイメージは、実は image-spec には完全に準拠していません。
Docker イメージは、`docker pull` してきた後に `docker save` などを実行することで tar 形式で書き出すことができます。少し中身を見てみましょう。

```sh
docker pull alpine:3.9
docker save alpine:3.9 --output alpine-3.9.tar

mkdir /root/alpine
tar -xf alpine-3.9.tar -C /root/alpine
```

```sh
ls /root/alpine
5cb3aa00f89934411ffba5c063a9bc98ace875d8f92e77d0029543d9f2ef4ad0.json  manifest.json
b40d48399b5890827b4252edbd2638b981772678bb1cc096436129f631722047       repositories
```

このように、`manifest.json` や `repositories` などが置かれており、
image-spec における [`image-layout.md`](https://github.com/opencontainers/image-spec/blob/v1.0.1/image-layout.md) に準拠していないことがわかります。
（`image-layout` では `index.json`, `oci-layout` という 2 ファイルと `blobs` というディレクトリが存在する必要があると書かれている）

さて、これを OCI 標準イメージに変換するために、[containers/skopeo](https://github.com/containers/skopeo) というツールを利用します。
皆さんにお配りした仮想マシンには既にインストール済みなので、次のように実行してみてください。

```sh
cd /root
skopeo copy docker://alpine:3.9 oci:alpine-oci:3.9
```

この状態で、`alpine-oci` ディレクトリを確認してみると、`image-layout` に準拠しているディレクトリ構造になっており、各ファイルも imege-spec に準拠しているものになります。

```sh
ls alpine-oci
blobs  index.json  oci-layout
```

このようにして、広く使われている Docker イメージを、image-spec に準拠したコンテナイメージに変換することができるようになっています。

## runtime-spec

[opencontainers/runtime-spec](https://github.com/opencontainers/runtime-spec) は、コンテナランタイムに関する標準化の文章です。
実際の仕様自体は [`spec.md`](https://github.com/opencontainers/runtime-spec/blob/v1.0.1/spec.md) に書かれています。

`linux`, `solaris`, `windows` など、ランタイムが動作する環境ごとに仕様が決定されています。ここでは `linux` の仕様について見ていきます。

## Filesystem bundle

コンテナを実際に実行する際に、必要な全てのデータとメタデータを含むようなファイルシステムの形式を、Filesystem bundle と呼んでいます。
構成は至ってシンプルで、ファイルシステムの種類にはよらず、次の 2 つで構成されているものです。

- `config.json`
    - bundle directory の root に `config.json` という名前で置かれなければならない
- コンテナのルートファイルシステム
    - 基本的には `config.json` に書かれている `root.path` によって参照されるディレクトリ

前述の [image-spec に書かれている通り](https://github.com/opencontainers/image-spec/tree/v1.0.1#running-an-oci-image)、
OCI イメージを仕様に基づいて解凍したものがこの Filesystem bundle になっています。

試しに、先ほど用意した image-spec に準拠したイメージを、Filesystem bundle に展開してみましょう。
展開には、[`oci-image-tool`](https://github.com/opencontainers/image-tools) というツールを利用できます。これも皆さんの仮想マシンには既にインストール済みになっています。

```sh
cd /root
mkdir alpine-bundle
oci-image-tool create --ref name=3.9 alpine-oci alpine-bundle
```

展開された `alpine-bundle` ディレクトリを眺めてみると、書かれている通りのファイルシステムツリーになっていることがわかります。

```json
ls alpine-bundle/
config.json  rootfs

cat alpine-bundle/config.json | jq .
{
  "ociVersion": "1.0.0",
  "process": {
    "terminal": true,
    "user": {
      "uid": 0,
      "gid": 0
    },
    "args": [
      "/bin/sh"
    ],
    "env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ],
    "cwd": "/"
  },
  "root": {
    "path": "rootfs"
  },
  "linux": {}
}
```

## Runtime and Lifecycle

実際に、コンテナランタイムの一連の動作を記述しているのが [`runtime.md`](https://github.com/opencontainers/runtime-spec/blob/v1.0.1/runtime.md) です。
コンテナランタイムは、コマンドラインツールになっており、以下のサブコマンドが利用可能になっている必要があります。

- `state <container-id>`
- `create <container-id>`
- `start <container-id>`
- `kill <container-id> <signal>`
- `delete <container-id>`

コンテナランタイムのライフサイクルでは、次のように動作が行われていきます。

- `create` サブコマンドが実行される
    - Filesystem bundle への参照と任意の ID が指定される
    - 指定された ID はコンテナの固有 ID となる
- `create` で作られるコンテナの実行環境は Filesystem bundle にある `config.json` の設定に従う
    - `config.json` に従えない場合はエラーを発生させる
    - `config.json` で指定したリソースが作成中の場合指定されているプログラムを実行してはならない
    - この手順のあとに Filesystem bundle に存在する `config.json` を変更してもコンテナには影響を及ぼさない
- `start` サブコマンドが実行される
    - 実行対象となるコンテナの固有 ID が指定される
- `config.json` に指定されている `.hooks.prestart` に書かれているコマンドが実行される
    - `.hooks.prestart` が失敗した場合にはコンテナを停止する必要がある
- `config.json` に指定されている `.process` に従ってプロセスが実行される
- `config.json` に指定されている `.hooks.poststart` に書かれているコマンドが実行される
    - `.hooks.poststart` が失敗した場合にはログに警告を出すが処理は継続する
- コンテナプロセスが終了する
    - これは処理の完了や、エラーや、`kill` などによって発生する
- `delete` サブコマンドが実行される
    - コンテナの固有 ID が渡される
- `create` で作成した全てのリソースを正しく元に戻す
- `config.json` に指定されている `.hooks.poststop` に書かれているコマンドが実行される
    - `.hooks.poststart` が失敗した場合にはログに警告を出すが処理は継続する

## runc コマンドを使ったコンテナの作成

それぞれ、実際に `runc` コマンドを実行して動作を確認してみましょう。
Filesystem bundle には先ほど作成した `alpine-bundle` を利用します。

```sh
cd /root/alpine-bundle
```

動作を簡潔にするために、`config.json` を次のものに置き換えます。
これは、`echo alpine` が実行されるような設定ファイルになっています。　

```json
{
  "ociVersion": "1.0.0",
  "process": {
    "terminal": false,
    "user": {
      "uid": 0,
      "gid": 0
    },
    "args": [
      "echo",
      "alpine"
    ],
    "env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ],
    "cwd": "/"
  },
  "root": {
    "path": "rootfs"
  },
  "linux": {},
  "mounts": [
    {
      "destination": "/proc",
      "type": "proc",
      "source": "proc"
    }
  ]
}
```

この状態で、`runc create <container-id>` を実行します。

```sh
runc create alpine-test
```

`runc list`, `runc state <container-id>` でコンテナの状態を確認します。

```sh
runc list
ID            PID         STATUS      BUNDLE                       CREATED                          OWNER
alpine-test   29017       created     /home/ubuntu/alpine-bundle   2019-03-18T22:18:31.693146066Z   root
```

```sh
runc state alpine-test
{
  "ociVersion": "1.0.1-dev",
  "id": "alpine-test",
  "pid": 29017,
  "status": "created",
  "bundle": "/home/ubuntu/alpine-bundle",
  "rootfs": "/home/ubuntu/alpine-bundle/rootfs",
  "created": "2019-03-18T22:18:31.693146066Z",
  "owner": ""
}
```

（この状態で `pid` のプロセスは一体どんな状態になっているでしょう？余裕があれば調べてみてください）

`runc start` で実際にコンテナを実行してみます。

```sh
runc start alpine-test
alpine
```

`config.json` に指定されている `args` 通り、`echo alpine` が実行され、`alpine` が出力されました。

また、`runc list`, `runc state <container-id>` で状態を確認してみましょう。

```sh
runc list
ID            PID         STATUS      BUNDLE                       CREATED                          OWNER
alpine-test   0           stopped     /home/ubuntu/alpine-bundle   2019-03-18T22:25:05.041029433Z   root
```

``sh
runc state alpine-test
{
  "ociVersion": "1.0.1-dev",
  "id": "alpine-test",
  "pid": 0,
  "status": "stopped",
  "bundle": "/home/ubuntu/alpine-bundle",
  "rootfs": "/home/ubuntu/alpine-bundle/rootfs",
  "created": "2019-03-18T22:25:05.041029433Z",
  "owner": ""
}`
``

このように、`STATUS` が `stopped` になっていることがわかります。
この状態のコンテナは Lifecycle に従って正しく `delete` を行う必要があるので、`runc delete <container-id>` を発行します。

```sh
runc delete alpine-test
```

`runc list` でコンテナがなくなったことを確認します。

```sh
runc list
ID          PID         STATUS      BUNDLE      CREATED     OWNER
```

細かい仕様や動作環境固有の設定などはたくさんありますが、`runtime-spec` で必須の仕様はここまで説明した Filesystem bundle, Runtime and Lifecycle がほとんど全てになっています。
