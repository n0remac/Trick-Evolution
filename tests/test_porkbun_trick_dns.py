import importlib.util
from pathlib import Path
import unittest


script = Path(__file__).resolve().parents[1] / "scripts/porkbun-trick-dns.py"
spec = importlib.util.spec_from_file_location("porkbun_trick_dns", script)
dns = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dns)


class TrickRecordTests(unittest.TestCase):
    def test_selects_only_trick_record(self):
        records = [
            {"id": "12", "name": "starcoaster.noremac.dev", "type": "A", "content": "165.227.55.254"},
            {"id": "42", "name": "trick.noremac.dev", "type": "A", "content": "146.190.175.159"},
        ]
        self.assertEqual(dns.trick_records(records)["id"], "42")

    def test_rejects_ambiguous_or_conflicting_records(self):
        old = {"id": "42", "name": "trick", "type": "A", "content": "146.190.175.159"}
        with self.assertRaises(ValueError):
            dns.trick_records([old, dict(old, id="43")])
        with self.assertRaises(ValueError):
            dns.trick_records([old, {"id": "44", "name": "trick", "type": "AAAA", "content": "::1"}])
        with self.assertRaises(ValueError):
            dns.trick_records([])


if __name__ == "__main__":
    unittest.main()
