import pathlib
import tempfile
import unittest
from unittest.mock import patch
import nginx_remote as nginx


class NginxSaveTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name).resolve()
        self.path = self.root / "app.conf"
        self.path.write_text("server { listen 80; }\n")
        self.old = self.path.read_bytes()
        self.scope = patch.multiple(nginx, CONFIG_ROOT=str(self.root), LOCK_PATH=str(self.root / "lock"))
        self.scope.start()
        self.addCleanup(self.scope.stop)
        self.addCleanup(self.temp.cleanup)

    def request(self, revision=None):
        return {"path": str(self.path), "update": {"revision": revision or nginx.digest(self.old), "text": "server { listen 8080; }\n"}}

    def test_success_and_read(self):
        read = nginx.operation({"path": str(self.path)})
        self.assertEqual(read["revision"], nginx.digest(self.old))
        with patch.object(nginx, "run", return_value=True) as run:
            result = nginx.operation(self.request())
        self.assertEqual(self.path.read_text(), result["text"])
        self.assertEqual(run.call_args_list[0].args[0], ["nginx", "-t"])
        self.assertEqual(run.call_args_list[1].args[0], ["systemctl", "reload", "nginx"])

    def test_stale_revision_never_writes(self):
        with patch.object(nginx, "run") as run:
            with self.assertRaisesRegex(ValueError, "已被修改"):
                nginx.operation(self.request("0" * 64))
            run.assert_not_called()
        self.assertEqual(self.path.read_bytes(), self.old)

    def test_validation_failure_restores_file(self):
        with patch.object(nginx, "run", side_effect=[False, True, True]):
            with self.assertRaisesRegex(ValueError, "校验失败"):
                nginx.operation(self.request())
        self.assertEqual(self.path.read_bytes(), self.old)

    def test_reload_failure_restores_file(self):
        with patch.object(nginx, "run", side_effect=[True, False, True, True]):
            with self.assertRaisesRegex(ValueError, "重载未成功"):
                nginx.operation(self.request())
        self.assertEqual(self.path.read_bytes(), self.old)

    def test_rejects_symlinks_and_outside_paths(self):
        link = self.root / "link.conf"
        link.symlink_to(self.path)
        with self.assertRaisesRegex(ValueError, "符号链接"):
            nginx.operation({"path": str(link)})
        with self.assertRaisesRegex(ValueError, "路径无效"):
            nginx.operation({"path": "/etc/passwd"})


if __name__ == "__main__":
    unittest.main()
