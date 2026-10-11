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


class TrackedEvent(dict):
    live_count = 0
    peak_live_count = 0

    def __init__(self, category, amount):
        super().__init__(category=category, amount=amount)
        type(self).live_count += 1
        type(self).peak_live_count = max(type(self).peak_live_count, type(self).live_count)

    def __del__(self):
        type(self).live_count -= 1


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

    def test_does_not_materialize_the_event_stream(self):
        TrackedEvent.live_count = 0
        TrackedEvent.peak_live_count = 0

        def stream():
            for index in range(256):
                yield TrackedEvent("docs", 1 if index % 2 == 0 else -1)

        summarize(OneShotEvents(stream()))

        self.assertLessEqual(TrackedEvent.peak_live_count, 4)


if __name__ == "__main__":
    unittest.main()
