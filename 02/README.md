# Linux プロセス型コンテナ入門

初期の Docker や、現在の Docker 標準コンテナランタイムである runc で用いられている、
Linux カーネルの機能を用いたコンテナの実装の概要について説明します。

## Linux とプロセス

Linux では、実行するタスクを「プロセス」と呼ばれる単位に分けて管理をしています。
Linux は、起動時に全てのプロセスの親となる `init` プロセスを起動し、`init` プロセスが更に必要なプロセスを起動していきます。

これは Linux 上で `pstree -p` などのコマンドを実行することで確認することが出来ます。
（なお、`/sbin/init` は 今回の環境では `/lib/systemd/systemd` へシンボリックリンクが貼られています）

Linux 環境において、ユーザがプロセスを新たに実行するときには、`fork` 並びに `exec` (`execve`) システムコールを利用するのが一般的です。

`fork` システムコールでは、実行元のプロセスを複製して新しいプロセスを生成します。
このとき、実行元のプロセスを親プロセス、複製された新しいプロセスを子プロセスと呼びます。

親プロセスと子プロセスは、自身の プロセス ID  や、自身の親プロセスの ID などのいくつかの点を除いて、全く同じものになっています。

`exec` (`execve`) システムコールでは、自身のプロセスを、実行するプログラムで上書きして実行します。

`exec` 系のシステムコールでは、実行した際にプロセスが置き換わってしまうため、プログラムが成功したか・失敗したかなどの判定を行うことができません。

そのため、Linux 環境の一般的なプロセスの起動では、`fork` を利用し、親プロセスから子プロセスを起動し、起動した子プロセスで `exec` を行い、
プロセス間通信に用いられる `pipe` を利用し、親プロセスと置き換わった後の子プロセスでやりとりを行う、といった方法がよく用いられます。

(refs: `man 2 fork`, `man 2 execve`, `man 3 exec`, `man 2 pipe`)

コンテナ環境では、起動しているコンテナがなるべく他のコンテナに影響を与えないように分離されていることが望ましいです。
通常の Linux のプロセスの起動では、親プロセスと資源を共有したり、他のプロセスの情報が見えたり、場合によっては操作してしまえたりします。

ここからは、コンテナの実装に必要な、親プロセスや他のプロセスと可能な限り分離して子プロセスを起動するための機能を紹介していきます。

## Linux Namespaces

Linux では、プロセスが使うリソースを分離して提供する、Namespaces という機能があります。
分離できるリソースは以下のとおりです。

- IPC
    - プロセス間通信で使うリソース(共有メモリ, セマフォ等)
- Network
    - ネットワークデバイスや IP アドレス、ルーティングテーブルなど
- Mount
    - ファイルシステムツリー
- PID
    - プロセス ID
- User
    - ユーザ ID / グループ ID
- UTS
    - nodename や domainname など

### 名前空間の分離

いくつかに関して、実際に試してみましょう。
次のような、`/bin/sh` を起動する際に名前空間を利用する Go のプログラムとして、`main.go` を用意します。
Go の `cmd.SysProcAttr` には、`clone(2)` に渡すのと同じような flags を渡すことができます。
(`clone(2)` は `fork(2)` と似たような子プロセスを生成するシステムコール)

試しに、IPC, Network, User に対して名前空間を利用するようにしてみます (`Cloneflags` に指定されている値に注目する)。

(refs: `man 2 clone`)

```go
// +build linux
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	cmd := exec.Command("/bin/sh")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWUSER,
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
```

このプログラムの `CloneFlags` があるものとないものを作成し、
それぞれを Linux 上で実行して、Namespace を設定する前と後で出力を比較してみてください。

- `ipcs`
- `ip addr show`
- `id`

### UID/GID の設定

User namespace を分離した後では、sh を実行したユーザ/グループが、nobody/nogroup になってしまっています。
新しい User 名前空間で実行されるプロセスの UID/GID を設定するためには、`/proc/[pid]/uid_map` と `/proc/[pid]/gid_map` に対して書き込みを行います。

(refs: `man 7 user_namespaces`)

Go の `Cmd.SysProcAttr` には、`CLONE_NEWUSER` した際の `UidMappings`, `GidMappings` を渡すことができ、
渡した値を `/proc/[pid]/uid_map` 並びに `/proc/[pid]/gid_map` に適切に書き込みを行ってくれるようになっています。

(refs: `https://github.com/golang/go/blob/go1.10.4/src/syscall/exec_linux.go#L438-L516`)

ここでは、プロセスを実行したユーザが、名前空間を分離した後のプロセスで uid/gid が 0 (root) になるように設定してみましょう。

```diff
--- 1.go	2019-03-19 13:46:27.000000000 +0900
+++ 2.go	2019-03-19 13:49:33.000000000 +0900
@@ -14,6 +14,20 @@
 		Cloneflags: syscall.CLONE_NEWIPC |
 			syscall.CLONE_NEWNET |
 			syscall.CLONE_NEWUSER,
+		UidMappings: []syscall.SysProcIDMap{
+			{
+				ContainerID: 0,
+				HostID:      os.Getuid(),
+				Size:        1,
+			},
+		},
+		GidMappings: []syscall.SysProcIDMap{
+			{
+				ContainerID: 0,
+				HostID:      os.Getgid(),
+				Size:        1,
+			},
+		},
 	}

 	cmd.Stdin = os.Stdin
```

このように変更を加えた後、`go run main.go` で実行したシェル内で `id` などを実行して、
正しく root として認識されていることを確認してください。

### UTS の設定

次に、hostname や domainname などを管理する UTS について見ていきます。

hostname を設定してからプロセスを起動するために、次のようなフローでプロセスの起動を行うことにします。

- プロセスの第一引数が `run` かどうかチェックする
    - `run` であれば Namespaces を設定しつつ第一引数を `init` に変えて自分自身を実行する
- プロセスの第一引数が `init` かどうかチェックする
    - `init` であれば hostname を設定した後に自分自身を `/bin/sh` に置き換える

このようにすることで、Namespaces が設定された後に hostname を設定しつつ `/bin/sh` を実行することができるようになります。
コードを見たほうが早いと思うので、実際に見てみましょう。

```go
// +build linux
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func Run() {
	cmd := exec.Command("/proc/self/exe", "init")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWUTS,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func InitContainer() error {
	if err := syscall.Sethostname([]byte("container")); err != nil {
		return fmt.Errorf("Setting hostname failed: %w", err)
	}
	if err := syscall.Exec("/bin/sh", []string{"/bin/sh"}, os.Environ()); err != nil {
		return fmt.Errorf("Exec failed: %w", err)
	}
	return nil
}

func Usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s run\n", os.Args[0])
	os.Exit(2)
}

func main() {
	if len(os.Args) <= 1 {
		Usage()
	}
	switch os.Args[1] {
	case "run":
		Run()
	case "init":
		if err := InitContainer(); err != nil {
			fmt.Fprintf(os.Stderr, "%+v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	default:
		Usage()
	}
}
```

このようにプログラムを変更した後、プログラムを実際に `go run main.go run` などで実行してみて、
`uname -n` コマンドの出力を確認してみましょう。

### PID と Mount

最後に、プロセス ID の分離とファイルシステムツリーの分離について見ていきます。

`cmd.SysProcAttr` を以下のように変更し、`syscall.CLONE_NEWPID` と `syscall.CLONE_NEWNS` を追加します。

```diff
--- 3.go	2019-03-19 13:53:22.000000000 +0900
+++ 4.go	2019-03-19 13:53:28.000000000 +0900
@@ -13,6 +13,8 @@
 	cmd.SysProcAttr = &syscall.SysProcAttr{
 		Cloneflags: syscall.CLONE_NEWIPC |
 			syscall.CLONE_NEWNET |
+			syscall.CLONE_NEWNS |
+			syscall.CLONE_NEWPID |
 			syscall.CLONE_NEWUSER |
 			syscall.CLONE_NEWUTS,
 		UidMappings: []syscall.SysProcIDMap{
```

`ps` コマンドなどが正しく分離された名前空間の情報を取得できるように、`/proc` ファイルシステムをマウントしてみましょう。
`InitContainer` 関数を次のように変更します。

```diff
--- 4.go	2019-03-19 13:54:30.000000000 +0900
+++ 5.go	2019-03-19 13:58:13.000000000 +0900
@@ -49,6 +49,9 @@
 	if err := syscall.Sethostname([]byte("container")); err != nil {
 		return fmt.Errorf("Setting hostname failed: %w", err)
 	}
+	if err := syscall.Mount("proc", "/proc", "proc", uintptr(syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV), ""); err != nil {
+		return fmt.Errorf("Proc mount failed: %w", err)
+	}
 	if err := syscall.Exec("/bin/sh", []string{"/bin/sh"}, os.Environ()); err != nil {
 		return fmt.Errorf("Exec failed: %w", err)
 	}
```

Go で `syscall.Mount` を呼び出す事でマウントを行えます。渡しているフラグに関しては `man 2 mount` を参考にしてみてください。

上記のように変更したあと、`go run main.go run` などでプログラムを実行し、以下のコマンドの実行結果を見てみましょう。

- `ps aufxw`
- `ls -asl /proc`

## chroot / pivot_root

ここまでで、Namespaces を用いたリソースの分離について見てきました。
利用するリソースは分離されましたが、ファイルシステムに関してはどうでしょうか？
このままでは、利用するファイルシステムは基本的に同じなため、あるコンテナが他のコンテナのファイルを読み書きすることができてしまいます。

Linux には、プロセスのルートディレクトリや、ルートファイルシステムを変更する `chroot` や `pivot_root` のような機能があります。
`InitContainer` 関数を次のように変更して、プロセスの実行時に `/root/chroot` をルートディレクトリにしてみましょう。

(refs: `man 2 chroot`)

```diff
--- 5.go	2019-03-19 13:58:13.000000000 +0900
+++ 6.go	2019-03-19 14:00:44.000000000 +0900
@@ -52,6 +52,12 @@
 	if err := syscall.Mount("proc", "/proc", "proc", uintptr(syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV), ""); err != nil {
 		return fmt.Errorf("Proc mount failed: %w", err)
 	}
+	if err := syscall.Chroot("/root/chroot"); err != nil {
+		return fmt.Errorf("Chroot failed: %w", err)
+	}
+	if err := os.Chdir("/"); err != nil {
+		return fmt.Errorf("Chdir failed: %w", err)
+	}
 	if err := syscall.Exec("/bin/sh", []string{"/bin/sh"}, os.Environ()); err != nil {
 		return fmt.Errorf("Exec failed: %w", err)
 	}
```

実行前に `/root/chroot` 並びに必要なディレクトリを作成しておきます。

```sh
mkdir -p /root/chroot/proc
```

また、chroot 後の環境でも `sh` と `ls` が利用できるように、`sh`, `ls` 並びに必要な静的ライブラリを設置します。

```sh
mkdir -p /root/chroot/bin
mkdir -p /root/chroot/lib

cp /bin/sh /root/chroot/bin
cp /bin/ls /root/chroot/bin

ldd /bin/sh
ldd /bin/ls

cp /lib/x86_64-linux-gnu/libc.so.6 /root/chroot/lib
cp /lib64/ld-linux-x86-64.so.2 /root/chroot/lib
cp /lib/x86_64-linux-gnu/libselinux.so.1 /root/chroot/lib
cp /lib/x86_64-linux-gnu/libpcre.so.3 /root/chroot/lib
cp /lib/x86_64-linux-gnu/libdl.so.2 /root/chroot/lib
cp /lib/x86_64-linux-gnu/libpthread.so.0 /root/chroot/lib

cd /root/chroot/
ln -s lib lib64

cd
```

上記のように変更したあと、`go run main.go run` などでプログラムを実行してみましょう。

### Escaping a chroot

さて、`chroot(2)` を利用してルートディレクトリを分離しました！これで実行されたプロセスからは上位のディレクトリが見えなくなって安全です！

...というのは本当でしょうか？

chroot するディレクトリである `/root/chroot/` に、`unchroot.go` という次のようなファイルを設置してみます。

```go
package main

import (
	"fmt"
	"os"
	"syscall"
)

func main() {
	if _, err := os.Stat(".42"); os.IsNotExist(err) {
		if err := os.Mkdir(".42", 0755); err != nil {
			fmt.Println("Mkdir failed")
		}
	}
	if err := syscall.Chroot(".42"); err != nil {
		fmt.Println("Chroot to .42 failed")
	}
	if err := syscall.Chroot("../../../../../../../../../../../../../../../.."); err != nil {
		fmt.Println("Jail break failed")
	}
	if err := syscall.Exec("/bin/sh", []string{""}, os.Environ()); err != nil {
		fmt.Println(err)
		fmt.Println("Exec failed")
	}
}
```

予めこのプログラムをビルドしておきます。

```sh
cd /root/chroot
go build unchroot.go

cd
```

このプログラムは、chroot されたディレクトリ内に単純に `.42` というディレクトリを作成し、まずそこに chroot します。
その後 、おもむろに上位のディレクトリを対象として、もう一度 `chroot` を行います。

`go run main.go run` などして、`/root/chroot` に chroot した `sh` を実行し、その後 `./unchroot` を実行してみましょう。
そして、`pwd` の出力や、`cd /` の結果、`ls` した結果などを見比べてみましょう。

これは、`main.go` で chroot したプロセスが、まだ `chroot(2)` を行う権限を持っているために発生しています。
そのため、上位のディレクトリを指定して `chroot(2)` をし直すことが可能になってしまっています。

これを避けるためには、後述する Linux capabilities の機能を利用して、プロセスが `chroot(2)` できないようにするか
`pivot_root` という、root ファイルシステムを変更するシステムコールを用いて同じような機能を実装することで解決できます。

### pivot_root

`main.go` を `chroot` ではなく `pivot_root` を用いた実装に変更してみましょう。

`pivot_root` で利用するディレクトリを `/root/rootfs` として `chroot` の時と同じように作成していきます。

```sh
mkdir -p /root/rootfs/proc
mkdir -p /root/rootfs/bin
mkdir -p /root/rootfs/lib

cp /bin/sh /root/rootfs/bin
cp /bin/ls /root/rootfs/bin

cp /lib/x86_64-linux-gnu/libc.so.6 /root/rootfs/lib
cp /lib64/ld-linux-x86-64.so.2 /root/rootfs/lib
cp /lib/x86_64-linux-gnu/libselinux.so.1 /root/rootfs/lib
cp /lib/x86_64-linux-gnu/libpcre.so.3 /root/rootfs/lib
cp /lib/x86_64-linux-gnu/libdl.so.2 /root/rootfs/lib
cp /lib/x86_64-linux-gnu/libpthread.so.0 /root/rootfs/lib

cd /root/rootfs/
ln -s lib lib64

cd
```

`main.go` を次のように変更します。

```diff
--- 6.go	2019-03-19 14:00:44.000000000 +0900
+++ 7.go	2019-03-19 14:10:29.000000000 +0900
@@ -49,11 +49,26 @@
 	if err := syscall.Sethostname([]byte("container")); err != nil {
 		return fmt.Errorf("Setting hostname failed: %w", err)
 	}
-	if err := syscall.Mount("proc", "/proc", "proc", uintptr(syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV), ""); err != nil {
+	if err := syscall.Mount("proc", "/root/rootfs/proc", "proc", uintptr(syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV), ""); err != nil {
 		return fmt.Errorf("Proc mount failed: %w", err)
 	}
-	if err := syscall.Chroot("/root/chroot"); err != nil {
-		return fmt.Errorf("Chroot failed: %w", err)
+	if err := os.Chdir("/root"); err != nil {
+		return fmt.Errorf("Chdir /root failed: %w", err)
+	}
+	if err := syscall.Mount("rootfs", "/root/rootfs", "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
+		return fmt.Errorf("Rootfs bind mount failed: %w", err)
+	}
+	if err := os.MkdirAll("/root/rootfs/oldrootfs", 0700); err != nil {
+		return fmt.Errorf("Oldrootfs create failed: %w", err)
+	}
+	if err := syscall.PivotRoot("rootfs", "/root/rootfs/oldrootfs"); err != nil {
+		return fmt.Errorf("PivotRoot failed: %w", err)
+	}
+	if err := syscall.Unmount("/oldrootfs", syscall.MNT_DETACH); err != nil {
+		return fmt.Errorf("Oldrootfs umount failed: %w", err)
+	}
+	if err := os.RemoveAll("/oldrootfs"); err != nil {
+		return fmt.Errorf("Remove oldrootfs failed: %w", err)
 	}
 	if err := os.Chdir("/"); err != nil {
 		return fmt.Errorf("Chdir failed: %w", err)
```

`pivot_root` を行うためには、いくつかの条件があります。

- `pivot_root(new_root, put_old)` では `new_root` と `put_old` が両方ディレクトリである必要がある
- `new_root` と `put_old` は `pivot_root` を実行するディレクトリと同じファイルシステムにあってはならない
    - この条件を満たすために、`/root/rootfs` を `rootfs` として `MS_BIND` を利用してマウントしています
- `put_old` は `new_root` 以下に存在しなければならない
- 他のファイルシステムが `put_old` にマウントされていてはならない

ここで出てくる `new_root`, `put_old` はコード中ではそれぞれ `/root/rootfs`, `/root/rootfs/oldrootfs` に対応しています。

(refs: `man 2 pivot_root`)

単純に `pivot_root` した後では、`/oldrootfs` というパスに元のファイルシステムがマウントされています。
元のファイルシステムにアクセスできないようにするために、`pivot_root` 行ったあとに、`/oldrootfs` をアンマウントし、`/oldrootfs` は不要なので削除しています。
（余力がある方は `/oldrootfs` をアンマウント・削除する部分のコードをコメントアウトしてみて、元のファイルシステムにアクセスできるか確認してみましょう）

また、事前に `proc` をマウントしておき、`rootfs` のマウント時に `MS_REC` を併せて付与することで、
`proc` がマウントされた状態のファイルシステムに `pivot_root` が行えるようになっています。

(refs: `man 2 mount`)

## capabilities

Linux のプロセスに対する権限チェックは、特権プロセスと呼ばれる、実効ユーザ ID (euid) が 0 （つまり root のこと）のプロセスか、 
非特権プロセスと呼ばれる実効ユーザ ID が 0 ではないプロセスかで大きく異なっており、特権プロセスでは全てのカーネルの権限チェックがバイパスされます。

Linux capabilities では、root が持っていた権限を capability と呼ばれるいくつかのグループに分割しています。
capability は、スレッド単位の属性であり、グループごとに独立に有効化・無効化を行えるようになっています。

例えば、"Escaping a chroot" の項で説明した `chroot` 環境からの脱獄は、実行するプロセスから `CAP_SYS_CHROOT` という capability を奪っておく事で回避できます。
（実行するプロセスが `chroot(2)` を発行できなくなるので他のディレクトリや上位のディレクトリに `chroot` し直されることがなくなる）

Go 言語から capabilities を操作するには、[syndtr/gocapability](https://github.com/syndtr/gocapability) などのライブラリを利用するのが簡単です。
（実際に `runc` でも gocapability を用いて capability の設定を行っています）

また、シェルから簡単に試すために、`capsh(1)` という capability を操作した上で `/bin/bash` を起動するプログラムも存在しています。
（発展：Ubuntu の環境で `capsh` を利用して capability を制限し、`chown` が行えないシェルを起動してみましょう ）

スレッドが実際にどのような capability を持つかは、実行ファイルについている File capability とスレッド自体の capability など、
複雑な要素によって決定されます。計算の方法や、実際にどのような capability があるかは、`man 7 capabilities` などを参考にしてください。

（発展: `syscall.Exec` で呼ぶコマンドを `capsh` 経由にすることで capabilities を設定しながらコマンドを実行して確認してみましょう）
（また、この方法での capability の制限にはどのような問題点があるか考えてみましょう）

## cgroups

ここまでで、プロセスを起動する際のリソースの分離について手を動かしながら見てきました。

しかし、プロセスが使う Linux 上のリソースが分離されていても、CPU やメモリなどの計算資源はまだ共有されてしまっています。
例えば、CPU を常にマシンの 100% 専有し続けるコンテナが起動していたら、他のコンテナや、ホストに影響を及ぼしてしまいます。

こういった際に利用できるのが、Linux に実装されている cgroups (Control groups) と呼ばれるプロセスの管理機構です。
cgroups では、プロセスをグループ単位でまとめ、そのグループ内のプロセスに対して、CPU やメモリなどの利用量などを制限することができるようになっています。

システムコールを用いて cgroups を操作する方法ももちろんありますが、今回は簡単のために、ホストでマウント済みの cgroupfs というファイルシステムを用いて、
プロセスにリソース制限を掛けてみましょう。

cgroupfs は、現在では一般的に `/sys/fs/cgroups` にマウントされており、このファイルシステムに対して読み込み・書き込みの操作を行うことで、
cgroups 内でのリソースの利用状況を確認したり、リソースの利用に制限を掛けることが可能になっています。
cgroup には v1 と v2 があり、v2 がもちろん推奨されているのですが、多くの環境でまだ v1 が使われているという事情もあり、今回は v1 の操作について説明します。
（とはいえ、v1 と v2 で今回説明する範囲での操作自体に大きな変わりはありません）

実際に `my-container` という名前の cpu レベルでの cgroup を作るには、以下のようにします。
　
```sh
mkdir /sys/fs/cgroup/cpu/my-container/
```

このようにすると、作成したディレクトリの配下に様々なファイルが現れます。

```sh
ls /sys/fs/cgroup/cpu/my-container/
cgroup.clone_children  cpu.cfs_quota_us  cpuacct.stat       cpuacct.usage_percpu       cpuacct.usage_sys   tasks
cgroup.procs           cpu.shares        cpuacct.usage      cpuacct.usage_percpu_sys   cpuacct.usage_user
cpu.cfs_period_us      cpu.stat          cpuacct.usage_all  cpuacct.usage_percpu_user  notify_on_release
```

どのプロセスをこの cgroup の管理下に入れるかというのを、`tasks` というファイルで管理しています。
例えば、自分自身が現在起動しているシェルをこの cgroup の管理下に入れたい場合は、次のように出来ます。

``sh
echo $$ > /sys/fs/cgroup/cpu/my-container/tasks
``

これで今起動しているシェルは `my-container` cgroup の管理下に入りました。

実際に CPU 制限を行ってみましょう。例えば `cpu.cfs_quota_us` という設定値は、`cpu.cfs_period_us` マイクロ秒間あたりに、何マイクロ秒間 CPU を利用できるか、という値です。

`cpu.cfs_period_us` のデフォルト値は、以下の通り 100000 マイクロ秒 (0.1 秒) になっています。

```sh
cat /sys/fs/cgroup/cpu/my-container/cpu.cfs_period_us
100000
```

そのため、CPU 使用率を 1% に制限したい場合は、`cpu.cfs_quota_us` に `1000` と書き込めば良いわけになります。

実際に、CPU 利用率を制限する前と後で、CPU をそれなりに利用するコマンド `yes >> /dev/null` を実行して眺めてみましょう。

```sh
yes >> /dev/null &     # バックグラウンドジョブとして yes >> /dev/null を起動
top                    # yes コマンドの CPU 使用率を眺めてみる

echo 1000 > /sys/fs/cgroup/cpu/my-container/cpu.cfs_quota_us
echo $(pgrep yes) > /sys/fs/cgroup/cpu/my-container/tasks

top                    # yes コマンドの CPU 使用率を眺めてみる
```

(refs: `man 7 cgroups`)

### 自作コンテナで cgroup を利用する

紹介した通り、cgroup の操作は、cgroupfs がマウントされていればファイルシステム操作で行えることがわかりました。
自作コンテナ上で、自分自身の CPU 使用率を 1% に制限した状態でシェルを起動するようにしてみましょう。

```diff
--- 7.go	2019-03-19 14:41:07.000000000 +0900
+++ 8.go	2019-03-20 05:50:22.000000000 +0900
@@ -3,6 +3,7 @@

 import (
 	"fmt"
+	"io/ioutil"
 	"os"
 	"os/exec"
 	"syscall"
@@ -49,7 +50,18 @@
 	if err := syscall.Sethostname([]byte("container")); err != nil {
 		return fmt.Errorf("Setting hostname failed: %w", err)
 	}
-	if err := syscall.Mount("proc", "/root/rootfs/proc", "proc", uintptr(syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV), ""); err != nil {
+
+	if err := os.MkdirAll("/sys/fs/cgroup/cpu/my-container", 0700); err != nil {
+		return fmt.Errorf("Cgroups namespace my-container create failed: %w", err)
+	}
+	if err := ioutil.WriteFile("/sys/fs/cgroup/cpu/my-container/tasks", []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644); err != nil {
+		return fmt.Errorf("Cgroups register tasks to my-container namespace failed: %w", err)
+	}
+	if err := ioutil.WriteFile("/sys/fs/cgroup/cpu/my-container/cpu.cfs_quota_us", []byte("1000\n"), 0644); err != nil {
+		return fmt.Errorf("Cgroups add limit cpu.cfs_quota_us to 1000 failed: %w", err)
+	}
+
+	if err := syscall.Mount("proc", "/root/rootfs/proc", "proc", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, ""); err != nil {
 		return fmt.Errorf("Proc mount failed: %w", err)
 	}
 	if err := os.Chdir("/root"); err != nil {
```

## その他のシステムコールや技術

ここで紹介した以外にも、`seccomp(2)` などを使ってコンテナから実行するシステムコールを制限したり、
overlayfs などを利用して、コンテナ内でのファイルの書き込みがホストの rootfs に影響しなくなるようにするなど、
実際に利用されているコンテナでは様々な技術が使われています。

ここまでで、namespace や chroot / pivot_root, capabilities, cgroups などを見てきたのと同じように、
一つ一つは Linux のカーネルや、ファイルシステムの技術を利用しているというのは共通しています。

基本的な調べ方なども共通しているはずなので、興味がある方はぜひ実際に使っている Docker などの実装についてより調べてみて貰えればと思います。
