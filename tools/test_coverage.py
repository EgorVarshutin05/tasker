"""A regression case for repeated -coverpkg blocks in separate test binaries."""
import json,subprocess,sys,tempfile,unittest
from pathlib import Path
class CoverageRegression(unittest.TestCase):
    def test_merge_and_threshold_exit(self):
        tool=Path(__file__).with_name('check_coverage.py')
        with tempfile.TemporaryDirectory() as directory:
            profile=Path(directory)/'coverage.out'
            profile.write_text('mode: atomic\n'
              'example.com/tasks/internal/service/service.go:1.1,2.1 2 1\n'
              'example.com/tasks/internal/service/service.go:1.1,2.1 2 0\n'
              'example.com/tasks/internal/repository/sqlite.go:1.1,2.1 3 1\n'
              'example.com/tasks/internal/handler/handler.go:1.1,2.1 5 1\n')
            result=subprocess.run([sys.executable,str(tool),str(profile)],capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            report=json.loads(profile.with_name('coverage-summary.json').read_text())
            self.assertEqual(report['total'],{'covered':10,'total':10,'percent':100.0})
            profile.write_text(profile.read_text().replace('3 1','3 0'))
            result=subprocess.run([sys.executable,str(tool),str(profile)],capture_output=True,text=True)
            self.assertEqual(result.returncode,1,result.stdout+result.stderr)
if __name__=='__main__':unittest.main()
