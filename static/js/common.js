function highlightCipher(id) {
	$("#cipher-list .cipher").removeClass("highlight");
	$(".cipher#cipher-" + id).addClass("highlight").scrollintoview();
}

// MAP FUNCTIONS:

if (typeof L !== 'undefined') {
	var teamIcon = L.ExtraMarkers.icon({
		icon: 'fa-street-view',
		shape: 'circle',
		markerColor: 'blue',
		svg: true,
		prefix: 'fa'
	});
	var cipherIcon = L.ExtraMarkers.icon({
		icon: 'fa-file',
		shape: 'penta',
		markerColor: 'yellow',
		svg: true,
		prefix: 'fa'
	});
}

// Function to generate strings like "za 15m 26s"
function component(x, v) { return Math.floor(x / v); }
function getCountdownText(finalDate, now) {
	var word = "za";
	var d = (finalDate - now) / 1000;
	if (d < 0) {
		word = "před"
		d *= -1;
	}

	var days    = component(d, 24 * 60 * 60),
	    hours   = component(d,      60 * 60),
	    minutes = component(d,           60) % 60,
	    seconds = component(d,            1) % 60;

	var out = word + " ";
	if (days >= 2) {
		out += days + " dny";
	} else if (hours >= 1) {
		out += hours + "h " + minutes + "m";
	} else {
		out += minutes + "m " + seconds + "s";
	}
	return out;
}

// Central time-update function
var toUpdateTimeElements = [];
setInterval(function() {
	var now = Date.now();
	toUpdateTimeElements.forEach(function(item) {
		$(item.el).html(getCountdownText(item.finalDate, now));
	});
}, 1000);

$(function(){
	$('[data-countdown]').each(function() {
		toUpdateTimeElements.push({
			finalDate: Date.parse($(this).data('countdown')),
			el: this,
		});
	});
	$('[data-countdown-title]').mouseover(function() {
		var $this = $(this)
		var finalDate = Date.parse($this.data('countdown-title'));
		var now = Date.now();
		$this.prop('title', getCountdownText(finalDate, now));
	});
});
