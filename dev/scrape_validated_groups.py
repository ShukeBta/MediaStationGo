"""One-shot client of the server's shared scrape preview/apply API.

Input: {"groups":[{"media_ids":["uuid"],"match":{"source":"tmdb",
"media_type":"tv","tmdb_id":123,"title":"Show"}}]}.
Default: preview only. --apply explicitly permits writing validated rows.
Never calls TMDB, 115, scans or filesystem APIs; no local matching rules.
"""
import argparse
import json
import os
from pathlib import Path
import urllib.request
import urllib.error


def save(path, value):
    fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, 'w', encoding='utf-8') as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.flush()
        os.fsync(stream.fileno())


def request(base, token, route, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base.rstrip('/') + '/api' + route, data=data,
                                 headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=330) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError('API HTTP %d: %s' % (error.code, error.read().decode())) from None


def prepare(preview, match):
    valid = [r for r in preview['items'] if r['valid']]
    return [r['media_id'] for r in valid], dict(match, expected_revisions={r['media_id']: r['revision'] for r in valid})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-url', required=True)
    parser.add_argument('--input', required=True)
    parser.add_argument('--output-dir', required=True)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    token = os.environ.get('MEDIASTATION_TOKEN')
    if not token:
        raise ValueError('MEDIASTATION_TOKEN is required; never put tokens in the input manifest')
    groups = json.loads(Path(args.input).read_text(encoding='utf-8'))['groups']
    output = Path(args.output_dir)
    output.mkdir(mode=0o700, parents=True, exist_ok=False)
    for index, group in enumerate(groups):
        preview = request(args.base_url, token, '/media/scrape/preview', dict(group, automatic_selection=True))
        save(output / ('%04d-preview.json' % index), preview)
        ids, match = prepare(preview, group['match'])
        print(json.dumps({'group': index, 'valid': len(ids), 'review': len(preview['items']) - len(ids)}), flush=True)
        if not args.apply or not ids:
            continue
        backup = [request(args.base_url, token, '/media/' + mid) for mid in ids]
        save(output / ('%04d-before.json' % index), backup)
        body = match if len(ids) == 1 else {'media_ids': ids, 'match': match}
        route = '/media/' + ids[0] + '/scrape/apply' if len(ids) == 1 else '/media/scrape/apply'
        try:
            result = request(args.base_url, token, route, body)
        except (RuntimeError, OSError) as error:
            save(output / ('%04d-error.json' % index), {'error': str(error), 'state': 'read back before any retry'})
            raise
        save(output / ('%04d-result.json' % index), result)
        after = [request(args.base_url, token, '/media/' + mid) for mid in ids]
        save(output / ('%04d-after.json' % index), after)
        expected = {r['media_id']: r for r in preview['items'] if r['valid']}
        for row in after:
            p = expected[row['id']]
            if (row.get('scrape_status') != 'matched' or row.get('tmdb_id') != match['tmdb_id']
                    or any(row.get(k, 0) != p.get(k, 0) for k in ('season_num', 'episode_num', 'episode_end_num', 'episode_part_num'))):
                raise RuntimeError('readback mismatch; inspect saved results before continuing')
        if len(ids) > 1 and (result.get('errors') or result.get('applied') != len(ids)):
            raise RuntimeError('partial batch; inspect results, no automatic retry')


if __name__ == '__main__':
    main()
