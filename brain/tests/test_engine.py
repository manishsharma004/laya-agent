import unittest

from laya_brain.engine import Engine, build_questions, control_question, extract_typed_text, extract_url


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
        self.calls = []

    def predict(self, state, questions):
        self.calls.append((state, questions))
        self.seen = (state, questions)
        if set(questions) == {"target"}:
            criteria = questions["target"]["criteria"]
            wanted = (self.answers.get("target") or {}).get("choice")
            choice = wanted if wanted in criteria else next(iter(criteria))
            return {"answers": {"target": {"choice": choice}}, "routing": {"model": "fake"}}
        if set(questions) == {"option"}:
            criteria = questions["option"]["criteria"]
            wanted = (self.answers.get("option") or {}).get("choice")
            choice = wanted if wanted in criteria else next(iter(criteria))
            return {"answers": {"option": {"choice": choice}}, "routing": {"model": "fake"}}
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
        self.assertIn("User request:", runner.calls[0][0])
        self.assertEqual(set(runner.calls[0][1]), {"intent", "operation", "safe_to_run", "fit"})
        self.assertEqual(len(runner.calls), 1)

    def test_operation_lists_page_controls(self):
        page = {
            "elements": [
                {"id": "bad id", "name": "Nope", "role": "link"},
                {"id": "e9", "name": "Gmail", "role": "link", "href": "https://mail.google.com/"},
                {"id": "e2", "name": "Search", "role": "textbox", "editable": True},
                {"id": "e4", "name": "Color", "role": "combobox"},
            ]
        }
        questions = build_questions(page)
        self.assertEqual(
            set(questions["operation"]["criteria"]),
            {"click", "type", "select", "scroll", "navigate", "extract", "done"},
        )
        clickable = control_question(page, "click")["criteria"]
        self.assertEqual(set(clickable), {"e9"})
        self.assertIn("Gmail", clickable["e9"])
        self.assertIn("mail.google.com", clickable["e9"])
        self.assertEqual(set(control_question(page, "type")["criteria"]), {"e2"})
        self.assertEqual(set(control_question(page, "select")["criteria"]), {"e4"})
        many = {"elements": [{"id": f"e{index}", "name": f"Link {index}", "role": "link", "tag": "a"} for index in range(1, 46)]}
        listed = control_question(many, "click")["criteria"]
        self.assertEqual(len(listed), 45)
        self.assertIn("e45", listed)

    def test_choice_of_a_control_is_that_action(self):
        self.engine._router = FakeRunner(
            {
                "intent": {"choice": "control_page"},
                "operation": {"choice": "click:e5"},
                "safe_to_run": {"noul": 0.9},
                "fit": {"score": 3.2},
            }
        )
        self.engine._status = "ready"
        page = {
            **PAGE,
            "elements": PAGE["elements"]
            + [{"id": "e5", "role": "link", "name": "Gmail", "tag": "a", "href": "https://mail.google.com/mail/", "editable": False}],
        }
        result = self.engine.decide({"prompt": "open gmail", "page": page})
        self.assertEqual(result["action"]["type"], "click")
        self.assertEqual(result["action"]["elementId"], "e5")

    def test_later_pass_picks_the_control_the_verb_needs(self):
        page = {
            **PAGE,
            "elements": PAGE["elements"]
            + [{"id": "e5", "role": "link", "name": "Gmail", "tag": "a", "href": "https://mail.google.com/mail/", "editable": False}],
        }
        self.engine._router = FakeRunner(
            {
                "intent": {"choice": "control_page"},
                "operation": {"choice": "click"},
                "target": {"choice": "e5"},
                "safe_to_run": {"noul": 0.9},
                "fit": {"score": 3.2},
            }
        )
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "click Login", "page": page})
        self.assertEqual(result["action"]["elementId"], "e5")
        self.assertEqual(len(self.engine._router.calls), 2)

    def test_menu_options_are_a_third_pass(self):
        page = {
            **PAGE,
            "elements": [
                element if element["id"] != "e4" else {**element, "options": ["Red", "Blue"]}
                for element in PAGE["elements"]
            ],
        }
        self.engine._router = FakeRunner(
            {
                "intent": {"choice": "control_page"},
                "operation": {"choice": "select"},
                "target": {"choice": "e4"},
                "option": {"choice": "Blue"},
                "safe_to_run": {"noul": 0.9},
                "fit": {"score": 3.0},
            }
        )
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "set the color", "page": page})
        self.assertEqual(result["action"]["type"], "select")
        self.assertEqual(result["action"]["elementId"], "e4")
        self.assertEqual(result["action"]["text"], "Blue")
        self.assertEqual([set(questions) for _state, questions in self.engine._router.calls], [
            {"intent", "operation", "safe_to_run", "fit"},
            {"option"},
        ])

    def test_done_answer_can_still_choose_a_link(self):
        page = {
            "elements": [
                {"id": "e5", "role": "link", "name": "Gmail", "tag": "a", "href": "https://mail.google.com/"},
                {"id": "e6", "role": "link", "name": "Images", "tag": "a"},
            ]
        }
        self.engine._router = FakeRunner(
            {
                "intent": {"choice": "answer"},
                "operation": {"choice": "done"},
                "target": {"choice": "e5"},
                "safe_to_run": {"noul": 0.9},
                "fit": {"score": 3.0},
            }
        )
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "open gmail", "page": page})
        self.assertEqual(result["action"], {"type": "click", "elementId": "e5", "text": "", "url": ""})
        self.assertIn("none", self.engine._router.calls[1][1]["target"]["criteria"])

    def test_abstaining_on_a_named_link_still_clicks_it(self):
        page = {
            "elements": [
                {"id": "e5", "role": "link", "name": "Gmail", "tag": "a", "href": "https://mail.google.com/"},
                {"id": "e6", "role": "link", "name": "Images", "tag": "a"},
            ]
        }
        self.engine._router = FakeRunner(
            {
                "intent": {"choice": "clarify"},
                "operation": {"choice": "done"},
                "target": {"choice": "none"},
                "safe_to_run": {"noul": 0.9},
                "fit": {"score": 2.0},
            }
        )
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "open gmail", "page": page})
        self.assertEqual(result["action"]["type"], "click")
        self.assertEqual(result["action"]["elementId"], "e5")

    def test_one_control_skips_the_second_pass(self):
        page = {"elements": [{"id": "e7", "role": "link", "name": "Only", "tag": "a"}]}
        runner = FakeRunner(
            {
                "intent": {"choice": "control_page"},
                "operation": {"choice": "click"},
                "safe_to_run": {"noul": 0.9},
                "fit": {"score": 3.0},
            }
        )
        self.engine._router = runner
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "go", "page": page})
        self.assertEqual(result["action"]["elementId"], "e7")
        self.assertEqual(len(runner.calls), 1)

    def test_long_control_list_is_grouped(self):
        class FirstChoice:
            def __init__(self):
                self.calls = 0

            def predict(self, state, questions):
                del state
                self.calls += 1
                answers = {}
                for key, question in questions.items():
                    if question.get("type") == "choice":
                        answers[key] = {"choice": next(iter(question["criteria"]))}
                    elif question.get("type") == "noul":
                        answers[key] = {"noul": 0.9}
                    else:
                        answers[key] = {"score": 3.0}
                return {"answers": answers, "routing": {"model": "fake"}}

        page = {"elements": [{"id": f"e{index}", "name": f"Link {index}", "role": "link", "tag": "a"} for index in range(1, 21)]}
        runner = FirstChoice()
        self.engine._router = runner
        self.engine._status = "ready"
        result = self.engine.decide({"prompt": "open something", "page": page})
        self.assertEqual(result["action"]["type"], "click")
        self.assertTrue(result["action"]["elementId"].startswith("e"))
        self.assertGreater(runner.calls, 2)

    def test_extractors(self):
        self.assertEqual(extract_typed_text('type "hi there" into Search'), "hi there")
        self.assertEqual(extract_url("go to https://example.com/a."), "https://example.com/a")
        self.assertEqual(extract_url("go to google.com"), "https://google.com")


if __name__ == "__main__":
    unittest.main()
