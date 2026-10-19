import unittest

from laya_brain.engine import Engine, build_questions, extract_typed_text, extract_url


PAGE = {
    "url": "https://example.com/account",
    "title": "Account",
    "elements": [
        {"id": "e1", "role": "button", "name": "Login", "tag": "button", "editable": False},
        {"id": "e2", "role": "textbox", "name": "Search", "tag": "input", "editable": True},
        {"id": "e3", "role": "button", "name": "Delete account", "tag": "button", "editable": False},
        {"id": "e4", "role": "combobox", "name": "Color", "tag": "select", "editable": False},
    ],
}


class FakeRunner:
    def __init__(self, answers):
        self.answers = answers
        self.seen = None

    def predict(self, state, questions):
        self.seen = (state, questions)
        return {"answers": self.answers, "routing": {"model": "fake"}}


class EngineTests(unittest.TestCase):
    def setUp(self):
        self.engine = Engine(mode="off")

    def test_click_login(self):
        result = self.engine.decide({"prompt": "click Login", "page": PAGE})
        self.assertEqual(result["backend"], "heuristic")
        self.assertEqual(result["action"], {"type": "click", "elementId": "e1", "text": "", "url": ""})
        self.assertFalse(result["needsConfirmation"])
        self.assertIn("Login", result["reply"])

    def test_type_into_search(self):
        result = self.engine.decide({"prompt": 'type "hello" into Search', "page": PAGE})
        self.assertEqual(result["action"]["type"], "type")
        self.assertEqual(result["action"]["elementId"], "e2")
        self.assertEqual(result["action"]["text"], "hello")

    def test_delete_needs_confirmation_until_confirmed(self):
        held = self.engine.decide({"prompt": "click Delete account", "page": PAGE})
        self.assertTrue(held["needsConfirmation"])
        self.assertEqual(held["action"]["elementId"], "e3")
        released = self.engine.decide({"prompt": "click Delete account", "page": PAGE, "confirmed": True})
        self.assertFalse(released["needsConfirmation"])
        self.assertEqual(released["action"]["type"], "click")

    def test_question_without_page_change(self):
        result = self.engine.decide({"prompt": "what is this page?", "page": PAGE})
        self.assertIsNone(result["action"])
        self.assertIn("Account", result["reply"])

    def test_navigate_only_with_url_from_prompt(self):
        result = self.engine.decide({"prompt": "open https://example.com/docs", "page": PAGE})
        self.assertEqual(result["action"]["type"], "navigate")
        self.assertEqual(result["action"]["url"], "https://example.com/docs")

    def test_open_named_control_clicks_it(self):
        page = {
            **PAGE,
            "elements": PAGE["elements"]
            + [{"id": "e5", "role": "link", "name": "Gmail", "tag": "a", "href": "https://mail.google.com/mail/", "editable": False}],
        }
        result = self.engine.decide({"prompt": "open gmail", "page": page})
        self.assertEqual(result["action"]["type"], "click")
        self.assertEqual(result["action"]["elementId"], "e5")

    def test_laya_navigate_answer_still_clicks_a_matching_control(self):
        page = {
            **PAGE,
            "elements": PAGE["elements"]
            + [{"id": "e5", "role": "link", "name": "Gmail", "tag": "a", "href": "https://mail.google.com/mail/", "editable": False}],
        }
        self.engine._router = FakeRunner(
            {
                "intent": {"choice": "control_page"},
                "operation": {"choice": "navigate"},
                "target": {"choice": "none"},
                "safe_to_run": {"noul": 0.9},
                "fit": {"score": 3.2},
            }
        )
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "open gmail", "page": page})
        self.assertEqual(result["backend"], "laya")
        self.assertEqual(result["action"]["type"], "click")
        self.assertEqual(result["action"]["elementId"], "e5")

    def test_missing_page_does_not_invent_a_click(self):
        result = self.engine.decide({"prompt": "click Login"})
        self.assertIsNone(result["action"])
        self.assertIn("extension", result["reply"].lower())

    def test_laya_runner_answers_are_projected(self):
        runner = FakeRunner(
            {
                "intent": {"choice": "control_page"},
                "operation": {"choice": "select"},
                "target": {"choice": "e4"},
                "safe_to_run": {"noul": 0.93},
                "fit": {"score": 3.6},
            }
        )
        self.engine._router = runner
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "choose Red in Color", "page": PAGE})
        self.assertEqual(result["backend"], "laya")
        self.assertEqual(result["action"]["type"], "select")
        self.assertEqual(result["action"]["elementId"], "e4")
        self.assertEqual(result["action"]["text"], "Red")
        self.assertIn("User request:", runner.seen[0])
        self.assertNotIn("target", runner.seen[1])
        self.assertEqual(set(runner.seen[1]), {"intent", "operation", "safe_to_run", "fit"})

    def test_questions_stay_static(self):
        first = build_questions({"elements": [{"id": "bad id", "name": "Nope"}, {"id": "e9", "name": "Ok", "role": "link"}]})
        second = build_questions(None)
        self.assertEqual(first, second)
        self.assertNotIn("target", first)
        self.assertEqual(set(first["operation"]["criteria"]), {"click", "type", "select", "scroll", "navigate", "extract", "done"})

    def test_extractors(self):
        self.assertEqual(extract_typed_text('type "hi there" into Search'), "hi there")
        self.assertEqual(extract_url("go to https://example.com/a."), "https://example.com/a")
        self.assertEqual(extract_url("go to google.com"), "https://google.com")


if __name__ == "__main__":
    unittest.main()
