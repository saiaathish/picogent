import unittest

from ledger import summarize


class OneShotEvents:
    def __init__(self, events):
        self._events = events
        self._iterated = False

    def __iter__(self):
        if self._iterated:
            raise AssertionError("events were traversed more than once")
        self._iterated = True
        return iter(self._events)


class SummarizeTests(unittest.TestCase):
    def test_one_pass_preserves_order_and_zero_total(self):
        rows = [
            {"category": "docs", "amount": 4},
            {"category": "tests", "amount": 7},
            {"category": "docs", "amount": -4},
        ]
        events = OneShotEvents(rows)

        self.assertEqual(
            summarize(events),
            [
                {"category": "docs", "total": 0},
                {"category": "tests", "total": 7},
            ],
        )

    def test_does_not_mutate_input_rows(self):
        rows = [{"category": "docs", "amount": 4}, {"category": "tests", "amount": 7}]
        before = [row.copy() for row in rows]

        summarize(rows)

        self.assertEqual(rows, before)


if __name__ == "__main__":
    unittest.main()
