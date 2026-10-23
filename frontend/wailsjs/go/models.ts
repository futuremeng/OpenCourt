export namespace domain {
	
	export class Citation {
	    SourceFile: string;
	    PageNum: number;
	    Snippet: string;
	    LawVersion: string;
	    LawRepealed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Citation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SourceFile = source["SourceFile"];
	        this.PageNum = source["PageNum"];
	        this.Snippet = source["Snippet"];
	        this.LawVersion = source["LawVersion"];
	        this.LawRepealed = source["LawRepealed"];
	    }
	}
	export class Conflict {
	    conflict_id: string;
	    fact_ids: string[];
	    conflict_point: string;
	    resolution_method?: string;
	    is_resolved: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Conflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conflict_id = source["conflict_id"];
	        this.fact_ids = source["fact_ids"];
	        this.conflict_point = source["conflict_point"];
	        this.resolution_method = source["resolution_method"];
	        this.is_resolved = source["is_resolved"];
	    }
	}
	export class Fact {
	    fact_id: string;
	    description: string;
	    state: string;
	    citation?: Citation;
	    related_element?: string;
	    conflict_id?: string;
	    assumption_source?: string;
	    visibility: string;
	    is_manual_confirmed: boolean;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Fact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fact_id = source["fact_id"];
	        this.description = source["description"];
	        this.state = source["state"];
	        this.citation = this.convertValues(source["citation"], Citation);
	        this.related_element = source["related_element"];
	        this.conflict_id = source["conflict_id"];
	        this.assumption_source = source["assumption_source"];
	        this.visibility = source["visibility"];
	        this.is_manual_confirmed = source["is_manual_confirmed"];
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace gate {
	
	export class Violation {
	    rule: string;
	    phrase: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Violation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.phrase = source["phrase"];
	        this.message = source["message"];
	    }
	}

}

export namespace redact {
	
	export class Action {
	    rule_id: string;
	    original_hash: string;
	    strategy: string;
	    restorable: boolean;
	    // Go type: time
	    timestamp: any;
	
	    static createFrom(source: any = {}) {
	        return new Action(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule_id = source["rule_id"];
	        this.original_hash = source["original_hash"];
	        this.strategy = source["strategy"];
	        this.restorable = source["restorable"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Result {
	    text: string;
	    actions: Action[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.actions = this.convertValues(source["actions"], Action);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

