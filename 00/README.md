# Setup

講義を開始する前に、講義で使うためのインスタンスにログインする準備をします。
皆さんには事前に、インスタンスの IP アドレスと、そのインスタンスにログインするための秘密鍵をお渡しします。

受け取った秘密鍵は、SSH ログインで利用可能なように、次の操作を行ってください。

```
$ chmod 600 container-internship-2019.pem
$ mv container-internship-2019.pem ~/.ssh
```

会社の Wi-Fi に接続の上、次のコマンドを実行してみてください。

```
$ ssh ${インスタンスの IP アドレス} -l ubuntu -i ~/.ssh/container-internship-2019.pem
```

上手くログインが成功すればセットアップは完了です。
