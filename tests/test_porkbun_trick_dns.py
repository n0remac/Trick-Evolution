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
        self.assertEqual(dns.trick_record(records)["id"], "42")

    def test_rejects_ambiguous_or_conflicting_records(self):
        old = {"id": "42", "name": "trick", "type": "A", "content": "146.190.175.159"}
        with self.assertRaises(ValueError):
            dns.trick_record([old, dict(old, id="43")])
        with self.assertRaises(ValueError):
            dns.trick_record([old, {"id": "44", "name": "trick", "type": "AAAA", "content": "::1"}])
        self.assertIsNone(dns.trick_record([]))

    def test_missing_record_uses_only_one_wildcard(self):
        wildcard = {"id": "9", "name": "*.noremac.dev", "type": "A", "content": "146.190.175.159"}
        self.assertEqual(dns.wildcard_address([wildcard]), "146.190.175.159")
        with self.assertRaises(ValueError):
            dns.wildcard_address([])
        with self.assertRaises(ValueError):
            dns.wildcard_address([wildcard, dict(wildcard, id="10")])


if __name__ == "__main__":
    unittest.main()
