import unittest
from scrape_validated_groups import prepare


class PreviewTests(unittest.TestCase):
    def test_only_server_validated_ids_are_applied(self):
        preview = {'items': [{'media_id': 'a', 'revision': 'v1', 'valid': True},
                             {'media_id': 'b', 'revision': 'v2', 'valid': False}]}
        ids, match = prepare(preview, {'tmdb_id': 12})
        self.assertEqual(ids, ['a'])
        self.assertEqual(match['expected_revisions'], {'a': 'v1'})


if __name__ == '__main__':
    unittest.main()
