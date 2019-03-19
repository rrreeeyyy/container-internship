# 自作コンテナランタイムの拡張

最後に、ここまでの知識を使って自作のコンテナランタイムを作ってみましょう。

例えば、サンプルコードを別の言語で書き換えてコンテナランタイムを作ってみたり、
今日説明した以上の OCI の仕様に準拠してみたり、よりたくさんの Capabilites や、Cgroups の機能を使ってみたり、
Linux のプロセス以外の実装方法でコンテナランタイムを実装したり、Docker から利用可能なように実装したりと、自由に拡張を行ってみてください。

## 課題例

課題が思いつかない人のために、いくつか案を書いておきます。ここにあるものでもここにないものでも自由に実装を行ってみてください。

また、実装しようとしているもののアイデアが不安という方や、何を実装していいか分からない方、
実装したいものはあるがどのように実装すればよいかわからない方などは、相談にのるので積極的に相談してみてください。

- Linux プロセス型コンテナ改良系
    - Go ではない別の言語でシステムコールを呼び Linux のプロセス型コンテナを実装してみる
    - Linux Capabilites をいくつか設定できるように改良してみる
    - Cgroups の機能をより使えるように改良してみる
    - `pivot_root` 時に `/proc/self/fd{/,/0,/1,/2}` を正しく `/dev/{fd,stdin,stdout,stderr}` などにマッピングしてみる
    - `pivot_root` 時に rootfs ではない任意のホストディレクトリを扱えるようにしてみる
        - Dokcer の `--mount` オプションのようなもの
    - `pivot_root` 時に cgroupsfs をマウントしてみる
    - alpine linux の Filesystem bundles の `rootfs` に自作コンテナで `pivot_root` してみる
        - `apk` で何かをインストールしてみたりして挙動を調べてまとめてみる
    - Mount 時に overlayfs などを利用するようにしてみる
        - コンテナで実行しているプロセス内でファイルシステムを破壊しても安全なようにしてみる
    - seccomp(2) を使ってプロセスが呼べるシステムコールを制限する
- OCI 準拠系
    - `config.json` を読み取って動作する自作コンテナを作ってみる
    - OCI ランタイムのサブコマンドを実装してみる
        - `state <container-id>`
        - `create <container-id>`
        - `start <container-id>`
        - `kill <container-id> <signal>`
        - `delete <container-id>`
    - Docker の `runtimes` に自作コンテナのバイナリを追加してみる
        - 自作コンテナのバイナリで `argv` を出力してみて調べてみる
        - `docker` がどのように `runc` を呼び出しているか調べてみる
        - `docker` が `runc create <container-id>` を呼んだ後にどのようなファイルを期待するか調べてみる
    - `image-spec` について読んでまとめてみる
- CRI (Container Runtime Interface) 調査系
    - （今回は紹介しなかった）Container Runtime Interface について調べてまとめてみる
