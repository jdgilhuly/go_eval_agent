// Package evaltest provides Go testing integration for the eval framework,
// allowing eval cases to be written and run as standard Go test functions.
//
// The package provides a Harness that wraps *testing.T and manages shared
// configuration (provider, system prompt, tools). Each eval case is run as
// a subtest via Harness.Run, receiving a TestCase with helpers for tool
// mocking, input execution, and assertion methods.
//
// Example usage:
//
//	func TestMyAgent(t *testing.T) {
//	    h := evaltest.New(t, evaltest.WithProvider(myProvider))
//	    h.Run("greet", func(tc *evaltest.TestCase) {
//	        tc.MockTool("lookup", "John Doe")
//	        output := tc.Input("Greet the user")
//	        tc.AssertOutputContains("John")
//	        tc.AssertToolCalled("lookup")
//	    })
//	}
package evaltest
