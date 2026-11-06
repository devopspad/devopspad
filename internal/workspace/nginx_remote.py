"""Invoked over verified SSH; JSON input/output only, no configuration in logs."""
import fcntl
import hashlib
import json
import os
import re
import stat
import subprocess
import sys
import tempfile

CONFIG_ROOT = "/etc/nginx/conf.d"
LOCK_PATH = "/run/lock/devopspad-nginx.lock"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def install(path, data, metadata):
    fd, temp = tempfile.mkstemp(prefix=".devopspad-nginx-", dir=os.path.dirname(path))
    try:
        with os.fdopen(fd, "wb") as stream:
            os.fchmod(stream.fileno(), stat.S_IMODE(metadata.st_mode))
            os.fchown(stream.fileno(), metadata.st_uid, metadata.st_gid)
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temp, path)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def run(command):
    return subprocess.run(command, stdout=subprocess.DEVNULL,
                          stderr=subprocess.DEVNULL, timeout=20).returncode == 0


def operation(request):
    path = request.get("path", "")
    if os.path.dirname(path) != CONFIG_ROOT or not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9._-]*\.conf", os.path.basename(path)):
        raise ValueError("Nginx 配置路径无效")
    if os.path.realpath(path) != path:
        raise ValueError("Nginx 配置必须为普通文件，不能通过符号链接访问")
    with open(path, "rb") as stream:
        metadata = os.fstat(stream.fileno())
        if not stat.S_ISREG(metadata.st_mode):
            raise ValueError("Nginx 配置必须为普通文件")
        old = stream.read((1 << 20) + 1)
    if len(old) > 1 << 20:
        raise ValueError("Nginx 配置超过 1 MiB")
    old.decode("utf-8")
    update = request.get("update")
    if update is None:
        return {"path": path, "text": old.decode(), "revision": digest(old)}
    new = update["text"].encode("utf-8")
    if len(new) > 1 << 20 or b"\0" in new:
        raise ValueError("Nginx 配置内容无效")
    # Serialize all DevOpsPad Nginx writers because nginx -t covers the whole host.
    with open(LOCK_PATH, "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        with open(path, "rb") as stream:
            current = stream.read((1 << 20) + 1)
        if digest(current) != update.get("revision") or current != old:
            raise ValueError("Nginx 配置已被修改，请保留草稿并刷新后重试")
        if new == old:
            return {"path": path, "text": old.decode(), "revision": digest(old), "message": "配置未变化"}
        install(path, new, metadata)
        try:
            if not run(["nginx", "-t"]):
                raise ValueError("Nginx 配置校验失败，已恢复原文件；请检查语法及服务器错误日志")
            if not run(["systemctl", "reload", "nginx"]):
                raise ValueError("Nginx 重载未成功，已恢复原文件；请检查服务器状态")
        except BaseException:
            install(path, old, metadata)
            if run(["nginx", "-t"]):
                run(["systemctl", "reload", "nginx"])
            raise
    return {"path": path, "text": new.decode(), "revision": digest(new), "message": "配置已保存，nginx -t 通过并已重载"}


def main():
    try:
        raw = sys.stdin.buffer.read((8 << 20) + 1)
        if len(raw) > 8 << 20:
            raise ValueError("请求超过大小限制")
        result = operation(json.loads(raw))
    except (UnicodeError, json.JSONDecodeError):
        result = {"error": "配置不是有效的 UTF-8 文本或请求格式无效"}
    except ValueError as error:
        result = {"error": str(error)}
    except BlockingIOError:
        result = {"error": "其他 Nginx 配置正在保存，请稍后重试"}
    except PermissionError:
        result = {"error": "没有读取或保存宿主机 Nginx 配置的权限"}
    except Exception:
        result = {"error": "Nginx 操作未确认，请刷新文件并检查服务器状态"}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
