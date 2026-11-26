import 'package:flutter_test/flutter_test.dart';
import 'package:pocketbase/pocketbase.dart';
import 'package:versionradar/format.dart';
import 'package:versionradar/status.dart';
import 'package:versionradar/widgets/field_form.dart';

void main() {
  group('versionsMatch', () {
    test('matches semver with or without v prefix and build metadata', () {
      expect(versionsMatch('v1.2.3', '', '1.2.3', ''), isTrue);
      expect(versionsMatch('1.2.3 (45)', '', 'v1.2.3', ''), isTrue);
      expect(versionsMatch('1.2.3+45', '', '1.2.3', ''), isTrue);
      expect(versionsMatch('1.2.0', '', '1.3.0', ''), isFalse);
    });

    test('matches commit SHA prefixes across deployed version and commit', () {
      expect(
        versionsMatch(
          'abcdef1',
          '',
          'v1.4.0',
          'abcdef1234567890abcdef1234567890abcdef12',
        ),
        isTrue,
      );
      expect(
        versionsMatch(
          'v42',
          'deadbeef1234',
          'deadbee',
          'deadbeef1234567890abcdef1234567890abcdef',
        ),
        isTrue,
      );
    });
  });

  group('evaluateRadarHealth', () {
    test('reports upToDate vs outdated when Git latest_version is set', () {
      final radar = RecordModel({
        'id': 'r1',
        'latest_version': 'v2.0.0',
        'latest_commit': 'abcdef123456',
      });
      final okDep = RecordModel({
        'id': 'd1',
        'status': 'success',
        'version': '2.0.0',
      });
      final oldDep = RecordModel({
        'id': 'd2',
        'status': 'success',
        'version': '1.9.0',
      });
      final buildingDep = RecordModel({
        'id': 'd3',
        'status': 'in_progress',
        'version': '2.0.0',
      });
      final failedDep = RecordModel({
        'id': 'd4',
        'status': 'failed',
        'version': '2.0.0',
      });

      expect(evaluateRadarHealth(radar, okDep), RadarHealth.upToDate);
      expect(evaluateRadarHealth(radar, oldDep), RadarHealth.outdated);
      expect(evaluateRadarHealth(radar, buildingDep), RadarHealth.inProgress);
      expect(evaluateRadarHealth(radar, failedDep), RadarHealth.failed);
      expect(evaluateRadarHealth(radar, null), RadarHealth.idle);
    });
  });

  group('DisplayFormat', () {
    test('formats versions, SHAs, UUIDs, and build numbers cleanly', () {
      final sha = DisplayFormat.version(
        'abcdef1234567890abcdef1234567890abcdef12',
      );
      expect(sha.primary, 'abcdef1');
      final build = DisplayFormat.version('1.4.2 (89)');
      expect(build.primary, '1.4.2');
      expect(build.meta, '89');
      final uuid = DisplayFormat.version(
        '550e8400-e29b-41d4-a716-446655440000',
      );
      expect(uuid.primary, '#550e8400');
    });

    test('humanizes provider stages and extracts cloudflare env/branch', () {
      expect(DisplayFormat.stage('READY_FOR_SALE'), 'Live on App Store');
      expect(DisplayFormat.stage('clone_repo'), 'Cloning Repo');
      final cf = DisplayFormat.deploymentMessage(
        'production main: Fix OAuth redirect\nExtra details',
        'cloudflare',
      );
      expect(cf.env, 'Production');
      expect(cf.branch, 'main');
      expect(cf.text, 'Fix OAuth redirect');
    });

    test('shortens git refs and extracts radar target labels', () {
      expect(DisplayFormat.shortRef('tag v1.2.3'), 'tag');
      expect(DisplayFormat.shortRef('branch main'), 'main');
      expect(DisplayFormat.shortRef('pubspec.yaml@main'), 'pubspec.yaml');
      final radar = RecordModel({
        'id': 'r1',
        'target': '{"app_name":"vr-api-prod"}',
        'source_target': '{"repo_url":"https://gitlab.com/dc-ds/vr.git"}',
      });
      expect(DisplayFormat.radarTargetLabel(radar), 'vr-api-prod');
      expect(DisplayFormat.sourceTargetLabel(radar), 'dc-ds/vr');
    });
  });

  group('FieldForm', () {
    test('respects optional fields and defaults', () {
      final form = FieldForm([
        {'key': 'branch', 'label': 'Branch', 'default': 'main'},
        {'key': 'prefix', 'label': 'Prefix', 'optional': true},
      ]);
      final (vals, err) = form.values();
      expect(err, isNull);
      expect(vals, {'branch': 'main'});
    });
  });
}
